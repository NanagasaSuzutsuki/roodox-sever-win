package serverapp

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestArtifactJanitorCleanupRespectsRetentionAndByteLimit(t *testing.T) {
	rootDir := t.TempDir()
	oldPath := filepath.Join(rootDir, "roodox-suite-old")
	midPath := filepath.Join(rootDir, "roodox-suite-mid")
	newPath := filepath.Join(rootDir, "roodox-suite-new")
	keepPath := filepath.Join(rootDir, "shared-hotspot")

	writeArtifactDir(t, oldPath, 8)
	writeArtifactDir(t, midPath, 8)
	writeArtifactDir(t, newPath, 8)
	writeArtifactDir(t, keepPath, 32)

	oldTime := time.Now().Add(-2 * time.Hour)
	setPathModTime(t, oldPath, oldTime)
	setPathModTime(t, midPath, time.Now().Add(-30*time.Minute))
	setPathModTime(t, newPath, time.Now().Add(-10*time.Minute))

	j := &artifactJanitor{
		rootDir:   rootDir,
		retention: time.Hour,
		maxBytes:  12,
		prefixes:  []string{"roodox-suite-"},
	}
	j.cleanup(time.Now())

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expected old artifact to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(midPath); !os.IsNotExist(err) {
		t.Fatalf("expected mid artifact to be removed by byte limit, stat err=%v", err)
	}
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("expected newest artifact to remain, stat err=%v", err)
	}
	if _, err := os.Stat(keepPath); err != nil {
		t.Fatalf("expected non-matching path to remain, stat err=%v", err)
	}
}

func TestConflictFileJanitorCleansRecursively(t *testing.T) {
	rootDir := t.TempDir()
	oldConflict := filepath.Join(rootDir, "nested", "demo.txt.roodox-conflict-20260401-010203.000000001-ab12cd")
	newConflict := filepath.Join(rootDir, "nested", "demo.txt.roodox-conflict-20260401-010203.000000002-bc23de")
	normalFile := filepath.Join(rootDir, "nested", "demo.txt")

	if err := os.MkdirAll(filepath.Dir(oldConflict), 0o755); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(oldConflict, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile(oldConflict) returned error: %v", err)
	}
	if err := os.WriteFile(newConflict, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile(newConflict) returned error: %v", err)
	}
	if err := os.WriteFile(normalFile, []byte("normal"), 0o644); err != nil {
		t.Fatalf("WriteFile(normalFile) returned error: %v", err)
	}
	if err := os.Chtimes(oldConflict, time.Now().Add(-2*time.Hour), time.Now().Add(-2*time.Hour)); err != nil {
		t.Fatalf("Chtimes(oldConflict) returned error: %v", err)
	}

	j := &conflictFileJanitor{
		rootDir:   rootDir,
		retention: time.Hour,
		maxBytes:  0,
	}
	j.cleanup(time.Now())

	if _, err := os.Stat(oldConflict); !os.IsNotExist(err) {
		t.Fatalf("expected old conflict file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(newConflict); err != nil {
		t.Fatalf("expected new conflict file to remain, stat err=%v", err)
	}
	if _, err := os.Stat(normalFile); err != nil {
		t.Fatalf("expected normal file to remain, stat err=%v", err)
	}
}

func TestLogFileJanitorUsesPatternsAndByteLimit(t *testing.T) {
	rootDir := t.TempDir()
	oldLog := filepath.Join(rootDir, "server.old.log")
	newLog := filepath.Join(rootDir, "server.new.log")
	otherFile := filepath.Join(rootDir, "notes.txt")

	if err := os.WriteFile(oldLog, []byte("12345678"), 0o644); err != nil {
		t.Fatalf("WriteFile(oldLog) returned error: %v", err)
	}
	if err := os.WriteFile(newLog, []byte("12345678"), 0o644); err != nil {
		t.Fatalf("WriteFile(newLog) returned error: %v", err)
	}
	if err := os.WriteFile(otherFile, []byte("keep"), 0o644); err != nil {
		t.Fatalf("WriteFile(otherFile) returned error: %v", err)
	}
	if err := os.Chtimes(oldLog, time.Now().Add(-30*time.Minute), time.Now().Add(-30*time.Minute)); err != nil {
		t.Fatalf("Chtimes(oldLog) returned error: %v", err)
	}

	j := &logFileJanitor{
		dir:       rootDir,
		retention: 0,
		maxBytes:  8,
		patterns:  []string{"server*.log"},
	}
	j.cleanup(time.Now())

	if _, err := os.Stat(oldLog); !os.IsNotExist(err) {
		t.Fatalf("expected old log to be removed by byte limit, stat err=%v", err)
	}
	if _, err := os.Stat(newLog); err != nil {
		t.Fatalf("expected new log to remain, stat err=%v", err)
	}
	if _, err := os.Stat(otherFile); err != nil {
		t.Fatalf("expected non-matching file to remain, stat err=%v", err)
	}
}

func TestArtifactJanitorTriggerCleansCreatedArtifact(t *testing.T) {
	rootDir := t.TempDir()
	j := newArtifactJanitor(rootDir, ArtifactCleanupRuntimeConfig{
		Enabled:   true,
		Interval:  0,
		Retention: time.Hour,
		MaxBytes:  0,
		Prefixes:  []string{"roodox-suite-"},
	})
	if j == nil {
		t.Fatal("expected artifact janitor to be created")
	}
	defer j.Close()

	oldPath := filepath.Join(rootDir, "roodox-suite-old")
	writeArtifactDir(t, oldPath, 8)
	setPathModTime(t, oldPath, time.Now().Add(-2*time.Hour))

	j.Trigger()
	waitForRemoval(t, oldPath, 2*time.Second)
}

func TestLogFileJanitorTriggerCleansCreatedLog(t *testing.T) {
	rootDir := t.TempDir()
	j := newLogFileJanitor(rootDir, LogCleanupRuntimeConfig{
		Enabled:   true,
		Interval:  0,
		Retention: time.Hour,
		MaxBytes:  0,
		Patterns:  []string{"server*.log"},
	})
	if j == nil {
		t.Fatal("expected log janitor to be created")
	}
	defer j.Close()

	oldLog := filepath.Join(rootDir, "server.old.log")
	if err := os.WriteFile(oldLog, []byte("12345678"), 0o644); err != nil {
		t.Fatalf("WriteFile(oldLog) returned error: %v", err)
	}
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldLog, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes(oldLog) returned error: %v", err)
	}

	j.Trigger()
	waitForRemoval(t, oldLog, 2*time.Second)
}

func writeArtifactDir(t *testing.T, path string, size int) {
	t.Helper()

	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%q) returned error: %v", path, err)
	}
	content := make([]byte, size)
	if err := os.WriteFile(filepath.Join(path, "artifact.bin"), content, 0o644); err != nil {
		t.Fatalf("WriteFile(%q) returned error: %v", path, err)
	}
}

func setPathModTime(t *testing.T, path string, ts time.Time) {
	t.Helper()

	if err := os.Chtimes(path, ts, ts); err != nil {
		t.Fatalf("Chtimes(%q) returned error: %v", path, err)
	}
	filePath := filepath.Join(path, "artifact.bin")
	if err := os.Chtimes(filePath, ts, ts); err != nil {
		t.Fatalf("Chtimes(%q) returned error: %v", filePath, err)
	}
}

func waitForRemoval(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("path %q was not removed within %v", path, timeout)
}
