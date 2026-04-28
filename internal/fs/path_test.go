package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathAllowsNestedFile(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")
	got, err := ResolvePath(root, filepath.Join("dir", "file.txt"))
	if err != nil {
		t.Fatalf("ResolvePath returned error: %v", err)
	}

	want := filepath.Join(root, "dir", "file.txt")
	if got != want {
		t.Fatalf("ResolvePath = %q, want %q", got, want)
	}
}

func TestResolvePathBlocksSiblingPrefixEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")

	_, err := ResolvePath(root, filepath.Join("..", "root-evil", "file.txt"))
	if err == nil {
		t.Fatal("ResolvePath unexpectedly allowed escaping sibling path")
	}
}

func TestNormalizeRelativePathCanonicalizesAliases(t *testing.T) {
	got, err := NormalizeRelativePath(filepath.Join("dir", ".", "child.txt"))
	if err != nil {
		t.Fatalf("NormalizeRelativePath returned error: %v", err)
	}
	if got != "dir/child.txt" {
		t.Fatalf("NormalizeRelativePath = %q, want %q", got, "dir/child.txt")
	}
}

func TestResolvePathRejectsAbsolutePathAsInvalidArgument(t *testing.T) {
	root := filepath.Join(t.TempDir(), "root")

	_, err := ResolvePath(root, root)
	if !errors.Is(err, os.ErrInvalid) {
		t.Fatalf("ResolvePath error = %v, want os.ErrInvalid", err)
	}
}
