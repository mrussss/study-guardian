package windows

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotatingFileWriter(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "studyguardian-log-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	logFile := filepath.Join(tempDir, "test.log")
	// Small maxBytes to test rotation easily (100 bytes)
	writer, err := NewRotatingFileWriter(logFile, 100, 3)
	if err != nil {
		t.Fatalf("failed to create rotating writer: %v", err)
	}
	defer writer.Close()

	// Write 60 bytes
	data1 := []byte("123456789012345678901234567890123456789012345678901234567890")
	_, err = writer.Write(data1)
	if err != nil {
		t.Fatalf("failed to write data1: %v", err)
	}

	// Write another 60 bytes (should trigger rotation)
	_, err = writer.Write(data1)
	if err != nil {
		t.Fatalf("failed to write data2: %v", err)
	}

	// Verify rotated file .1 exists
	if _, err := os.Stat(logFile + ".1"); err != nil {
		t.Fatalf("expected rotated log file .1 to exist, got: %v", err)
	}
}
