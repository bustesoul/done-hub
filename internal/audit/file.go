package audit

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// FileWriter is only called by Recorder's worker. Opening files, creating
// directories, rotation and compression never run on a request goroutine.
type FileWriter struct {
	log            *lumberjack.Logger
	ready          bool
	archive        *archiveWriter
	archiveEnabled bool
	closed         bool
}

func NewFileWriter(filename string) *FileWriter {
	return &FileWriter{
		log:     &lumberjack.Logger{Filename: filename, MaxSize: 100, MaxAge: 7, MaxBackups: 10},
		archive: newArchiveWriter(filepath.Join(filepath.Dir(filename), "archive")),
	}
}

// Called only by Recorder's worker; changing the hot configuration itself does
// not open files, acquire filesystem locks or wait for outstanding writes.
func (w *FileWriter) SetArchiveEnabled(enabled bool) { w.archiveEnabled = enabled }

func (w *FileWriter) Write(data []byte) (int, error) {
	if w.closed {
		return 0, os.ErrClosed
	}
	if w.archiveEnabled {
		return w.archive.Write(data)
	}
	if err := w.archive.seal(); err != nil {
		return 0, err
	}
	if !w.ready {
		if err := os.MkdirAll(filepath.Dir(w.log.Filename), 0700); err != nil {
			return 0, err
		}
		file, err := os.OpenFile(w.log.Filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err != nil {
			return 0, err
		}
		err = file.Chmod(0600)
		closeErr := file.Close()
		if err != nil {
			return 0, err
		}
		if closeErr != nil {
			return 0, closeErr
		}
		w.ready = true
	}
	return w.log.Write(data)
}

func (w *FileWriter) Maintain(now time.Time) error {
	if w.closed {
		return nil
	}
	// Recover a crashed process's files even if no new audited request arrives,
	// or the operator has since switched back to normal mode. Do not create an
	// archive directory merely because the maintenance timer is running.
	if !w.archive.initialized {
		if _, err := os.Stat(w.archive.dir); err == nil {
			if err := w.archive.initialize(); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if !w.archiveEnabled || w.archive.size >= w.archive.maxBytes || (!w.archive.openedAt.IsZero() && now.Sub(w.archive.openedAt) >= w.archive.interval) {
		return w.archive.seal()
	}
	return nil
}

func (w *FileWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	return errors.Join(w.archive.Close(), w.log.Close())
}
