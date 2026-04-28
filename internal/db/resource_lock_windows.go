//go:build windows

package db

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type resourceLock interface {
	Release() error
}

type windowsResourceLock struct {
	handle windows.Handle
	path   string
}

func acquireResourceLock(resourcePath string) (resourceLock, error) {
	absPath, err := filepath.Abs(resourcePath)
	if err != nil {
		return nil, err
	}

	lockPath := absPath + ".lock"
	ptr, err := windows.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, err
	}

	handle, err := windows.CreateFile(
		ptr,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		0,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("resource lock already held for %s: %w", absPath, err)
	}

	return &windowsResourceLock{
		handle: handle,
		path:   lockPath,
	}, nil
}

func (l *windowsResourceLock) Release() error {
	if l == nil || l.handle == 0 {
		return nil
	}

	handle := l.handle
	path := l.path
	l.handle = 0
	l.path = ""

	if err := windows.CloseHandle(handle); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
