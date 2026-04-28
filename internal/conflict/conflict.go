package conflict

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"
)

var fallbackSuffixCounter uint64

// GenerateConflictPath generates a conflict file path like:
// "proj/main.cpp.roodox-conflict-20250101-120000.123456789-ab12cd".
func GenerateConflictPath(path string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	now := time.Now()
	ts := fmt.Sprintf("%s.%09d", now.Format("20060102-150405"), now.Nanosecond())
	name := fmt.Sprintf("%s.roodox-conflict-%s-%s", base, ts, randomSuffix())
	if dir == "." || dir == string(filepath.Separator) {
		return name
	}
	return filepath.Join(dir, name)
}

func randomSuffix() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}

	fallback := atomic.AddUint64(&fallbackSuffixCounter, 1)
	return fmt.Sprintf("%06x", fallback&0xffffff)
}
