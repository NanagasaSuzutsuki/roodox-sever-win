// internal/fs/hash.go
package fs

import (
	"crypto/sha256"
	"encoding/hex"
)

// ComputeHash 计算 sha256，返回 hex 字符串
func ComputeHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
