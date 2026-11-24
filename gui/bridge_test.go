package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebrisServiceIsDirectory(t *testing.T) {
	svc := NewDebrisService(nil)
	tmpDir := t.TempDir()

	isDir, err := svc.IsDirectory(tmpDir)
	if err != nil {
		t.Fatalf("IsDirectory(%q) returned unexpected error: %v", tmpDir, err)
	}
	if !isDir {
		t.Fatalf("IsDirectory(%q) = false, want true", tmpDir)
	}

	filePath := filepath.Join(tmpDir, "sample.txt")
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	isDir, err = svc.IsDirectory(filePath)
	if err != nil {
		t.Fatalf("IsDirectory(%q) returned unexpected error: %v", filePath, err)
	}
	if isDir {
		t.Fatalf("IsDirectory(%q) = true, want false", filePath)
	}

	missingPath := filepath.Join(tmpDir, "missing")
	if _, err := svc.IsDirectory(missingPath); err == nil {
		t.Fatalf("IsDirectory(%q) expected error for missing path, got nil", missingPath)
	}
}
