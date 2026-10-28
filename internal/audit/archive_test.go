package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func archiveFiles(t *testing.T, dir, suffix string) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "audit-*"+suffix))
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func fileBytes(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestArchiveDefaultsAndSizeSealing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "audit.log")
	w := NewFileWriter(path)
	if w.archive.maxBytes != 100<<20 || w.archive.interval != 5*time.Minute {
		t.Fatal("unexpected archive limits")
	}
	w.SetArchiveEnabled(true)
	if err := w.Maintain(time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Dir(path)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("empty archive mode created files")
	}
	w.archive.maxBytes = 6
	for i := 0; i < 3; i++ {
		if _, err := w.Write([]byte("{}\n")); err != nil {
			t.Fatal(err)
		}
	}
	if len(archiveFiles(t, w.archive.dir, ".log")) != 1 || len(archiveFiles(t, w.archive.dir, ".open")) != 1 {
		t.Fatal("size rollover did not separate active and sealed files")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if len(archiveFiles(t, w.archive.dir, ".open")) != 0 {
		t.Fatal("shutdown did not seal active file")
	}
	var total int
	for _, path := range archiveFiles(t, w.archive.dir, ".log") {
		info, _ := os.Stat(path)
		if info.Size() > 6 || info.Mode().Perm() != 0600 {
			t.Fatal("archive size or privacy", info)
		}
		total += len(fileBytes(t, path))
	}
	if total != 9 {
		t.Fatal("size rollover lost bytes", total)
	}
	info, _ := os.Stat(w.archive.dir)
	if info.Mode().Perm() != 0700 {
		t.Fatal("archive directory is not private")
	}
}

func TestArchiveIdleRotationAndModeSwitchPreservePending(t *testing.T) {
	w := NewFileWriter(filepath.Join(t.TempDir(), "audit.log"))
	now := time.Now()
	w.archive.now = func() time.Time { return now }
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("old archive\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Maintain(now.Add(5 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	sealed := archiveFiles(t, w.archive.dir, ".log")
	if len(sealed) != 1 || len(archiveFiles(t, w.archive.dir, ".open")) != 0 {
		t.Fatal("idle timer did not seal without a new write")
	}
	old := time.Now().Add(-30 * 24 * time.Hour)
	if err := os.Chtimes(sealed[0], old, old); err != nil {
		t.Fatal(err)
	}
	// Normal-mode lumberjack cleanup must have no access to the archive folder.
	w.log.MaxSize, w.log.MaxAge, w.log.MaxBackups = 1, 1, 1
	w.SetArchiveEnabled(false)
	if _, err := w.Write(bytes.Repeat([]byte("x"), 600<<10)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(bytes.Repeat([]byte("y"), 600<<10)); err != nil {
		t.Fatal(err)
	}
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("new archive\n")); err != nil {
		t.Fatal(err)
	}
	w.SetArchiveEnabled(false)
	if err := w.Maintain(now); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if string(fileBytes(t, sealed[0])) != "old archive\n" || len(archiveFiles(t, w.archive.dir, ".log")) != 2 {
		t.Fatal("mode change removed or overwrote pending archives")
	}
}

func TestArchiveCrashHelper(t *testing.T) {
	path := os.Getenv("DONEHUB_ARCHIVE_CRASH_TEST")
	if path == "" {
		return
	}
	w := NewFileWriter(path)
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("{\"before_crash\":true}\npartial")); err != nil {
		t.Fatal(err)
	}
	os.Exit(0) // Deliberately skip Close, just as an abrupt process exit does.
}

func TestArchiveRestartRecoversOpenFilesEvenWithoutNewRequests(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	command := exec.Command(os.Args[0], "-test.run=^TestArchiveCrashHelper$")
	command.Env = append(os.Environ(), "DONEHUB_ARCHIVE_CRASH_TEST="+path)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatal(err, string(output))
	}
	w := NewFileWriter(path)
	if len(archiveFiles(t, w.archive.dir, ".open")) != 1 {
		t.Fatal("crash fixture missing")
	}
	// Archive is now disabled, but prior bytes must still become eligible for backup.
	if err := w.Maintain(time.Now()); err != nil {
		t.Fatal(err)
	}
	paths := archiveFiles(t, w.archive.dir, ".log")
	if len(paths) != 1 || string(fileBytes(t, paths[0])) != "{\"before_crash\":true}\npartial" {
		t.Fatal("crash recovery truncated old bytes")
	}
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("new process\n")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if len(archiveFiles(t, w.archive.dir, ".log")) != 2 {
		t.Fatal("restart reused the previous filename")
	}
}

func TestArchiveRejectsSecondWriterAndNeverOverwritesSealedTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w := NewFileWriter(path)
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("original\n")); err != nil {
		t.Fatal(err)
	}
	other := NewFileWriter(path)
	other.SetArchiveEnabled(true)
	if _, err := other.Write([]byte("other\n")); err == nil {
		t.Fatal("second process could recover an active writer's file")
	}
	_ = other.Close()
	active := w.archive.path
	sealed := strings.TrimSuffix(active, ".open") + ".log"
	if err := os.WriteFile(sealed, []byte("do not overwrite"), 0600); err != nil {
		t.Fatal(err)
	}
	w.SetArchiveEnabled(false)
	if err := w.Maintain(time.Now()); err == nil {
		t.Fatal("existing sealed filename was overwritten")
	}
	if string(fileBytes(t, active)) != "original\n" || string(fileBytes(t, sealed)) != "do not overwrite" {
		t.Fatal("failed publication lost existing bytes")
	}
	if err := os.Remove(sealed); err != nil {
		t.Fatal(err)
	}
	if err := w.Maintain(time.Now()); err != nil {
		t.Fatal("publication could not retry", err)
	}
	if string(fileBytes(t, sealed)) != "original\n" {
		t.Fatal("publication retry changed bytes")
	}
	_ = w.Close()
}

func TestArchiveConcurrentHotConfigAndWorkerShutdown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	w := NewFileWriter(path)
	w.archive.maxBytes = 300
	s := NewService()
	r := NewRecorder(w)
	s.SetRecorder(r)
	s.Apply(Config{ArchiveEnabled: true})
	// archive_enabled selects storage only; it must never enable empty filters.
	if s.Status().Enabled {
		t.Fatal("archive flag enabled auditing without a filter")
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			s.Apply(Config{TokenIDs: []int{7}, ArchiveEnabled: i%2 == 0})
		}
	}()
	for i := 0; i < 300; i++ {
		if !r.enqueue(Event{Type: "test", RequestID: fmt.Sprint(i)}) {
			t.Fatal("unexpected queue drop")
		}
	}
	wg.Wait()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil || r.Status().WriteErrors != 0 {
		t.Fatal(err, r.Status())
	}
	paths := archiveFiles(t, w.archive.dir, ".log")
	if _, err := os.Stat(path); err == nil {
		paths = append(paths, path)
	}
	seen := map[string]bool{}
	for _, path := range paths {
		for _, line := range bytes.Split(bytes.TrimSpace(fileBytes(t, path)), []byte("\n")) {
			var event Event
			if err := json.Unmarshal(line, &event); err != nil || seen[event.RequestID] {
				t.Fatal("partial or repeated event", err, string(line))
			}
			seen[event.RequestID] = true
		}
	}
	if len(seen) != 300 {
		t.Fatal("hot switching lost queued events", len(seen))
	}
}

func TestArchiveFailureDoesNotBlockConfigOrShutdown(t *testing.T) {
	w := &blockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	s := NewService()
	r := NewRecorder(w)
	s.SetRecorder(r)
	r.enqueue(Event{Type: "blocked"})
	<-w.entered
	done := make(chan struct{})
	go func() { s.Apply(Config{TokenIDs: []int{7}, ArchiveEnabled: true}); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("archive configuration waited for disk")
	}
	close(w.release)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
	badPath := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(badPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	failing := NewFileWriter(filepath.Join(badPath, "audit.log"))
	r = NewRecorder(failing)
	s.SetRecorder(r)
	r.enqueue(Event{Type: "failed"})
	if err := r.Close(ctx); err != nil || r.Status().WriteErrors == 0 || r.Status().LastError == "" {
		t.Fatal("archive I/O failure was not contained", err, r.Status())
	}
}

func TestArchiveWorkerSealsWhileIdleAndReportsCloseErrors(t *testing.T) {
	w := NewFileWriter(filepath.Join(t.TempDir(), "audit.log"))
	w.archive.interval = time.Millisecond
	r := NewRecorder(w)
	s := NewService()
	s.SetRecorder(r)
	s.Apply(Config{TokenIDs: []int{7}, ArchiveEnabled: true})
	r.enqueue(Event{Type: "idle", RequestID: "timer"})
	deadline := time.Now().Add(3 * time.Second)
	for len(archiveFiles(t, w.archive.dir, ".log")) != 1 {
		if time.Now().After(deadline) {
			t.Fatal("worker did not seal while idle", r.Status())
		}
		time.Sleep(10 * time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil || r.Status().WriteErrors != 0 {
		t.Fatal(err, r.Status())
	}
	// Make the final publication fail and ensure the failure remains visible
	// after the worker exits, without removing either existing file.
	w = NewFileWriter(filepath.Join(t.TempDir(), "audit.log"))
	w.SetArchiveEnabled(true)
	if _, err := w.Write([]byte("pending\n")); err != nil {
		t.Fatal(err)
	}
	active := w.archive.path
	sealed := strings.TrimSuffix(active, ".open") + ".log"
	if err := os.WriteFile(sealed, []byte("existing\n"), 0600); err != nil {
		t.Fatal(err)
	}
	r = NewRecorder(w)
	if err := r.Close(ctx); err != nil || r.Status().WriteErrors != 1 {
		t.Fatal("close error was lost", err, r.Status())
	}
	if string(fileBytes(t, active)) != "pending\n" || string(fileBytes(t, sealed)) != "existing\n" {
		t.Fatal("failed shutdown discarded bytes")
	}
}

type closeBlockingWriter struct {
	entered chan struct{}
	release chan struct{}
}

func (w *closeBlockingWriter) Write(data []byte) (int, error) { return len(data), nil }
func (w *closeBlockingWriter) Close() error {
	close(w.entered)
	<-w.release
	return nil
}

func TestArchiveWriterCloseIsInsideShutdownDeadline(t *testing.T) {
	w := &closeBlockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	r := NewRecorder(w)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	if err := r.Close(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("writer close escaped shutdown deadline", err)
	}
	cancel()
	<-w.entered
	close(w.release)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := r.Close(ctx); err != nil {
		t.Fatal(err)
	}
}
