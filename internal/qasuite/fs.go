package qasuite

import (
	"fmt"
	"os"
	"path/filepath"
)

func EnsureDir(rootDir, rel string) (string, error) {
	full, err := ResolveRunRoot(rootDir, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(full, 0o755); err != nil {
		return "", err
	}
	return full, nil
}

func WriteFixtureFile(rootDir, rel, content string) error {
	full, err := ResolveRunRoot(rootDir, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return err
	}
	return os.WriteFile(full, []byte(content), 0o644)
}

func RemoveRunRoot(rootDir, rel string) error {
	full, err := ResolveRunRoot(rootDir, rel)
	if err != nil {
		return err
	}
	if err := os.RemoveAll(full); err != nil {
		return fmt.Errorf("remove %q failed: %w", full, err)
	}
	return nil
}

func EnsureCMakeBuildUnit(rootDir, rel string, body string) error {
	unitDir, err := EnsureDir(rootDir, rel)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(unitDir, "CMakeLists.txt"), []byte(body), 0o644)
}
