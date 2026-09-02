package windows

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

type RotatingFileWriter struct {
	mu         sync.Mutex
	filename   string
	maxBytes   int64
	maxBackups int
	current    *os.File
	written    int64
}

func NewRotatingFileWriter(filename string, maxBytes int64, maxBackups int) (*RotatingFileWriter, error) {
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024 // 10MB default
	}
	if maxBackups <= 0 {
		maxBackups = 5
	}

	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log dir: %w", err)
	}

	w := &RotatingFileWriter{
		filename:   filename,
		maxBytes:   maxBytes,
		maxBackups: maxBackups,
	}

	if err := w.openCurrent(); err != nil {
		return nil, err
	}

	return w, nil
}

func (w *RotatingFileWriter) openCurrent() error {
	info, err := os.Stat(w.filename)
	if err == nil {
		w.written = info.Size()
	} else {
		w.written = 0
	}

	f, err := os.OpenFile(w.filename, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	w.current = f
	return nil
}

func (w *RotatingFileWriter) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.written+int64(len(p)) > w.maxBytes {
		_ = w.rotate()
	}

	if w.current == nil {
		if err := w.openCurrent(); err != nil {
			return 0, err
		}
	}

	n, err = w.current.Write(p)
	w.written += int64(n)
	return n, err
}

func (w *RotatingFileWriter) rotate() error {
	if w.current != nil {
		_ = w.current.Close()
		w.current = nil
	}

	// Shift backups
	for i := w.maxBackups - 1; i >= 1; i-- {
		oldPath := fmt.Sprintf("%s.%d", w.filename, i)
		newPath := fmt.Sprintf("%s.%d", w.filename, i+1)
		if _, err := os.Stat(oldPath); err == nil {
			_ = os.Rename(oldPath, newPath)
		}
	}

	// Rename current to .1
	if _, err := os.Stat(w.filename); err == nil {
		_ = os.Rename(w.filename, fmt.Sprintf("%s.1", w.filename))
	}

	w.written = 0
	return w.openCurrent()
}

func (w *RotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.current != nil {
		err := w.current.Close()
		w.current = nil
		return err
	}
	return nil
}

// MultiWriter creates a writer that writes to both stdout/stderr and rotating file
func SetupLogger(logPath string) (io.WriteCloser, error) {
	return NewRotatingFileWriter(logPath, 10*1024*1024, 5)
}
