package audit

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Only the Recorder worker uses this writer. Published .log files are immutable
// and are never removed here, even after archive mode is turned off. The host
// backup job is their sole cleanup owner.
type archiveWriter struct {
	dir         string
	file        *os.File
	path        string
	size        int64
	openedAt    time.Time
	lock        *os.File
	initialized bool
	maxBytes    int64
	interval    time.Duration
	now         func() time.Time
}

func newArchiveWriter(dir string) *archiveWriter {
	return &archiveWriter{dir: dir, maxBytes: 100 << 20, interval: 5 * time.Minute, now: time.Now}
}

func (w *archiveWriter) initialize() error {
	if w.initialized {
		return nil
	}
	if err := os.MkdirAll(w.dir, 0700); err != nil {
		return err
	}
	if w.lock == nil {
		lock, err := lockArchiveDirectory(filepath.Join(w.dir, ".writer.lock"))
		if err != nil {
			return err
		}
		w.lock = lock
	}
	// Holding the directory lock proves these are from a previous process.
	// Recover exact bytes, including a possible partial final event after a
	// crash; never truncate an old file or reuse its name.
	paths, err := filepath.Glob(filepath.Join(w.dir, "audit-*.open"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if err := publishArchive(path); err != nil {
			return fmt.Errorf("恢复审计归档: %w", err)
		}
	}
	w.initialized = true
	return nil
}

func (w *archiveWriter) Write(data []byte) (int, error) {
	if len(data) == 0 {
		return 0, nil
	}
	if int64(len(data)) > w.maxBytes {
		return 0, fmt.Errorf("审计事件超过归档文件大小限制")
	}
	if err := w.initialize(); err != nil {
		return 0, err
	}
	now := w.now()
	if w.path != "" && (w.file == nil || w.size+int64(len(data)) > w.maxBytes || now.Sub(w.openedAt) >= w.interval) {
		if err := w.seal(); err != nil {
			return 0, err
		}
	}
	if w.file == nil {
		file, err := os.CreateTemp(w.dir, "audit-"+now.UTC().Format("20060102T150405.000000000Z")+"-*.open")
		if err != nil {
			return 0, err
		}
		w.file, w.path, w.openedAt, w.size = file, file.Name(), now, 0
	}
	n, err := w.file.Write(data)
	w.size += int64(n)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return n, err
}

func (w *archiveWriter) seal() error {
	if w.path == "" {
		return nil
	}
	if w.file != nil {
		err := w.file.Sync()
		closeErr := w.file.Close()
		w.file = nil
		if err = errors.Join(err, closeErr); err != nil {
			return err // Keep .open for a later retry or restart recovery.
		}
	}
	if err := publishArchive(w.path); err != nil {
		return err
	}
	w.path, w.size, w.openedAt = "", 0, time.Time{}
	return nil
}

func (w *archiveWriter) Close() error {
	err := w.seal()
	if w.lock != nil {
		err = errors.Join(err, w.lock.Close())
		w.lock = nil
	}
	return err
}

func publishArchive(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("归档活动文件不是普通文件: %s", path)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	err = file.Sync()
	closeErr := file.Close()
	if err = errors.Join(err, closeErr); err != nil {
		return err
	}
	if info.Size() == 0 {
		return os.Remove(path) // A failed first write must not create an empty archive.
	}
	sealed := strings.TrimSuffix(path, ".open") + ".log"
	if _, err = os.Lstat(sealed); !errors.Is(err, os.ErrNotExist) {
		if err == nil {
			err = fmt.Errorf("归档目标已存在: %s", sealed)
		}
		return err
	}
	// Unique names plus the exclusive writer lock prevent another writer from
	// creating this target. The host job only reads/removes sealed files.
	return os.Rename(path, sealed)
}
