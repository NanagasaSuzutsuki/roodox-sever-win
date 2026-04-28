package analyze

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzerSkipsIgnoredProjectArtifacts(t *testing.T) {
	root := t.TempDir()

	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatalf("MkdirAll(app) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "app", "CMakeLists.txt"), []byte("cmake_minimum_required(VERSION 3.20)\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(app/CMakeLists.txt) returned error: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(root, ".git", "hooks"), 0o755); err != nil {
		t.Fatalf("MkdirAll(.git/hooks) returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "hooks", "Makefile"), []byte("all:\n\t@echo ignored\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(.git/hooks/Makefile) returned error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "app", "Makefile.roodox-conflict-20260322-120102.123456789-ab12cd"), []byte("all:\n\t@echo ignored\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(conflict makefile) returned error: %v", err)
	}

	units, err := NewAnalyzer(root).Scan(".")
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if len(units) != 1 {
		t.Fatalf("Scan units length = %d, want 1", len(units))
	}
	if units[0].Path != "app" || units[0].Type != "cmake" {
		t.Fatalf("unexpected unit: %+v", units[0])
	}
}

func TestAnalyzerRejectsEscapingRoot(t *testing.T) {
	root := t.TempDir()

	_, err := NewAnalyzer(root).Scan(filepath.Join("..", "outside"))
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("Scan error = %v, want os.ErrInvalid", err)
	}
}
