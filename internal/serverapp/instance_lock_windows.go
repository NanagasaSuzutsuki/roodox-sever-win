//go:build windows

package serverapp

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

type instanceLock interface {
	Release() error
}

type windowsInstanceLock struct {
	handle windows.Handle
	path   string
}

func acquireInstanceLock(resourcePath string) (instanceLock, error) {
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
		return nil, fmt.Errorf("database instance lock already held for %s: %w", absPath, err)
	}

	return &windowsInstanceLock{
		handle: handle,
		path:   lockPath,
	}, nil
}

func (l *windowsInstanceLock) Release() error {
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
