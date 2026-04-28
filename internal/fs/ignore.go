// internal/fs/ignore.go
package fs

import (
	"path/filepath"
	"strings"
)

// ShouldIgnore 判断某个路径是否应该在同步/列表中忽略。
// 这里只做最简单的规则，后面可以改成读取 .roodoxignore / .gitignore。
func ShouldIgnore(path string) bool {
	base := filepath.Base(path)

	// 忽略隐藏文件和常见系统垃圾
	if strings.HasPrefix(base, ".") {
		switch base {
		case ".git", ".svn", ".DS_Store":
			return true
		}
	}

	// IDE 临时文件
	if strings.HasSuffix(base, "~") ||
		strings.HasSuffix(base, ".tmp") ||
		strings.HasSuffix(base, ".swp") {
		return true
	}

	return false
}

// IsConflictPath reports whether the path is a Roodox-generated conflict copy.
func IsConflictPath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.Contains(base, ".roodox-conflict-")
}

// ShouldIgnoreInProjectScan is for tree scans and isolated build copies.
// It keeps user-visible conflict files accessible in normal listings, but avoids
// treating them as regular project inputs during analysis/build staging.
func ShouldIgnoreInProjectScan(path string) bool {
	return ShouldIgnore(path) || IsConflictPath(path)
}

// IsTextFile 仅通过扩展名粗略判断是否为文本文件。
func IsTextFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".txt", ".md", ".log", ".c", ".cpp", ".h", ".hpp",
		".py", ".go", ".java", ".json", ".yaml", ".yml",
		".xml", ".ini", ".cfg", ".sh", ".bat", ".cs":
		return true
	default:
		return false
	}
}

// IsBinaryFile 与 IsTextFile 反向（非常粗糙，仅占位）
func IsBinaryFile(path string) bool {
	return !IsTextFile(path)
}
