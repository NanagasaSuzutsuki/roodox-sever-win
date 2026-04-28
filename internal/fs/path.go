package fs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func NormalizeRelativePath(rel string) (string, error) {
	if rel == "" || rel == "." || rel == "/" {
		return ".", nil
	}

	cleanRel := filepath.Clean(rel)
	if filepath.IsAbs(cleanRel) {
		return "", fmt.Errorf("%w: absolute path not allowed", os.ErrInvalid)
	}
	if cleanRel == ".." || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("%w: path escapes root", os.ErrInvalid)
	}
	if cleanRel == "." {
		return ".", nil
	}
	return filepath.ToSlash(cleanRel), nil
}

func NormalizeProjectPath(root, rel string) (string, error) {
	full, err := ResolvePath(root, rel)
	if err != nil {
		return "", err
	}

	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootAbs = filepath.Clean(rootAbs)

	normalized, err := filepath.Rel(rootAbs, full)
	if err != nil {
		return "", err
	}
	return NormalizeRelativePath(normalized)
}

func ResolvePath(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootAbs = filepath.Clean(rootAbs)

	cleanRel, err := NormalizeRelativePath(rel)
	if err != nil {
		return "", err
	}
	if cleanRel == "." {
		return rootAbs, nil
	}

	full := filepath.Clean(filepath.Join(rootAbs, filepath.FromSlash(cleanRel)))
	if !IsWithinRoot(rootAbs, full) {
		return "", fmt.Errorf("%w: path escapes root", os.ErrInvalid)
	}
	return full, nil
}

func IsWithinRoot(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}

	rootAbs = filepath.Clean(rootAbs)
	candidateAbs = filepath.Clean(candidateAbs)
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}

	rel = filepath.Clean(rel)
	if rel == ".." {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
