// internal/fs/filesystem.go
package fs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

type FileSystem struct {
	RootDir string
}

func NewFileSystem(root string) *FileSystem {
	// 规范化 root 路径
	abs, err := filepath.Abs(root)
	if err == nil {
		root = abs
	}
	return &FileSystem{RootDir: root}
}

// resolvePath: 把相对路径变成安全的绝对路径，并防止越出 root.
func (f *FileSystem) resolvePath(rel string) (string, error) {
	return ResolvePath(f.RootDir, rel)
}

func (f *FileSystem) ListDir(rel string) ([]os.DirEntry, error) {
	dir, err := f.resolvePath(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadDir(dir)
}

func (f *FileSystem) Stat(rel string) (os.FileInfo, error) {
	p, err := f.resolvePath(rel)
	if err != nil {
		return nil, err
	}
	return os.Stat(p)
}

func (f *FileSystem) ReadFile(rel string) ([]byte, error) {
	p, err := f.resolvePath(rel)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p)
}

func (f *FileSystem) WriteFile(rel string, data []byte, perm os.FileMode) error {
	p, err := f.resolvePath(rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, data, perm)
}

func (f *FileSystem) ReadAt(rel string, offset, length int64) ([]byte, int64, error) {
	if offset < 0 {
		return nil, 0, errors.New("offset must be >= 0")
	}

	p, err := f.resolvePath(rel)
	if err != nil {
		return nil, 0, err
	}

	file, err := os.Open(p)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	st, err := file.Stat()
	if err != nil {
		return nil, 0, err
	}
	size := st.Size()

	if offset >= size {
		return []byte{}, size, nil
	}

	n := size - offset
	if length > 0 && length < n {
		n = length
	}

	buf := make([]byte, n)
	readN, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, 0, err
	}
	return buf[:readN], size, nil
}

func (f *FileSystem) WriteAt(rel string, offset int64, data []byte, perm os.FileMode) (int64, int64, error) {
	if offset < 0 {
		return 0, 0, errors.New("offset must be >= 0")
	}

	p, err := f.resolvePath(rel)
	if err != nil {
		return 0, 0, err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return 0, 0, err
	}

	file, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return 0, 0, err
	}
	defer file.Close()

	n, err := file.WriteAt(data, offset)
	if err != nil {
		return 0, 0, err
	}

	st, err := file.Stat()
	if err != nil {
		return 0, 0, err
	}
	return int64(n), st.Size(), nil
}

func (f *FileSystem) SetFileSize(rel string, size int64, perm os.FileMode) (int64, error) {
	if size < 0 {
		return 0, errors.New("size must be >= 0")
	}

	p, err := f.resolvePath(rel)
	if err != nil {
		return 0, err
	}

	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return 0, err
	}

	file, err := os.OpenFile(p, os.O_RDWR|os.O_CREATE, perm)
	if err != nil {
		return 0, err
	}
	defer file.Close()

	if err := file.Truncate(size); err != nil {
		return 0, err
	}

	st, err := file.Stat()
	if err != nil {
		return 0, err
	}
	return st.Size(), nil
}

func (f *FileSystem) Remove(rel string) error {
	p, err := f.resolvePath(rel)
	if err != nil {
		return err
	}
	return os.Remove(p)
}

func (f *FileSystem) Mkdir(rel string, perm os.FileMode) error {
	p, err := f.resolvePath(rel)
	if err != nil {
		return err
	}
	return os.MkdirAll(p, perm)
}

func (f *FileSystem) Rename(oldRel, newRel string) error {
	oldPath, err := f.resolvePath(oldRel)
	if err != nil {
		return err
	}
	newPath, err := f.resolvePath(newRel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}
