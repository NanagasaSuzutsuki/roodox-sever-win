//go:build !windows

package serverapp

import (
	"fmt"
	"os"
	"path/filepath"
)

type instanceLock interface {
	Release() error
}

type fileInstanceLock struct {
	path string
}

func acquireInstanceLock(resourcePath string) (instanceLock, error) {
	absPath, err := filepath.Abs(resourcePath)
	if err != nil {
		return nil, err
	}

	lockPath := absPath + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("database instance lock already held for %s: %w", absPath, err)
	}
	_ = file.Close()
	return &fileInstanceLock{path: lockPath}, nil
}

func (l *fileInstanceLock) Release() error {
	if l == nil || l.path == "" {
		return nil
	}
	path := l.path
	l.path = ""
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
