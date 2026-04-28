package server

import "roodox_server/internal/fs"

func normalizeProjectPath(root, path string) (string, error) {
	return fs.NormalizeProjectPath(root, path)
}

func normalizeRelativePath(path string) (string, error) {
	return fs.NormalizeRelativePath(path)
}
