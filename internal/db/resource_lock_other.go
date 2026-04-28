//go:build !windows

package db

import (
	"fmt"
	"os"
	"path/filepath"
)

type resourceLock interface {
	Release() error
}

type fileResourceLock struct {
	path string
}

func acquireResourceLock(resourcePath string) (resourceLock, error) {
	absPath, err := filepath.Abs(resourcePath)
	if err != nil {
		return nil, err
	}

	lockPath := absPath + ".lock"
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("resource lock already held for %s: %w", absPath, err)
	}
	_ = file.Close()
	return &fileResourceLock{path: lockPath}, nil
}

func (l *fileResourceLock) Release() error {
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
