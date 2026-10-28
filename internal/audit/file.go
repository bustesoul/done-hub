package audit

import (
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"
)

// FileWriter is only called by Recorder's worker. Opening files, creating
// directories, rotation and compression never run on a request goroutine.
type FileWriter struct {
	log   *lumberjack.Logger
	ready bool
}

func NewFileWriter(filename string) *FileWriter {
	return &FileWriter{log: &lumberjack.Logger{Filename: filename, MaxSize: 100, MaxAge: 7, MaxBackups: 10}}
}

func (w *FileWriter) Write(data []byte) (int, error) {
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

func (w *FileWriter) Close() error { return w.log.Close() }
