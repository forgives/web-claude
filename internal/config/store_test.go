package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkingDirDefaultsToBaseDir(t *testing.T) {
	baseDir := t.TempDir()
	store := &Store{}

	got, err := store.WorkingDir(baseDir)
	if err != nil {
		t.Fatalf("WorkingDir returned error: %v", err)
	}
	if got != baseDir {
		t.Fatalf("WorkingDir = %q, want %q", got, baseDir)
	}
}

func TestWorkingDirResolvesRelativePath(t *testing.T) {
	baseDir := t.TempDir()
	workingDir := filepath.Join(baseDir, "workspace")
	if err := os.Mkdir(workingDir, 0o755); err != nil {
		t.Fatalf("Mkdir workingDir: %v", err)
	}

	store := &Store{
		cfg: File{WorkingDir: "workspace"},
	}

	got, err := store.WorkingDir(baseDir)
	if err != nil {
		t.Fatalf("WorkingDir returned error: %v", err)
	}
	if got != workingDir {
		t.Fatalf("WorkingDir = %q, want %q", got, workingDir)
	}
}

func TestWorkingDirRejectsMissingDirectory(t *testing.T) {
	baseDir := t.TempDir()
	store := &Store{
		cfg: File{WorkingDir: "missing"},
	}

	if _, err := store.WorkingDir(baseDir); err == nil {
		t.Fatal("expected error for missing working_dir")
	}
}

func TestWorkingDirRejectsFile(t *testing.T) {
	baseDir := t.TempDir()
	filePath := filepath.Join(baseDir, "not-a-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
		t.Fatalf("WriteFile filePath: %v", err)
	}

	store := &Store{
		cfg: File{WorkingDir: filePath},
	}

	if _, err := store.WorkingDir(baseDir); err == nil {
		t.Fatal("expected error for file working_dir")
	}
}
