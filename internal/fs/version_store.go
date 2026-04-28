// internal/fs/version_store.go
package fs

import "sync"

// VersionRecord 保存某个文件的单条历史记录。
type VersionRecord struct {
	Version    uint64
	MtimeUnix  int64
	Hash       string
	Size       int64
	ClientID   string
	ChangeType string // create / modify / delete
}

// VersionStore 管理所有文件的历史版本（当前内存实现，后面可换 SQLite）
type VersionStore struct {
	mu      sync.RWMutex
	history map[string][]VersionRecord // key = path
}

func NewVersionStore() *VersionStore {
	return &VersionStore{
		history: make(map[string][]VersionRecord),
	}
}

func (s *VersionStore) Append(path string, rec VersionRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history[path] = append(s.history[path], rec)
}

func (s *VersionStore) GetHistory(path string) []VersionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]VersionRecord(nil), s.history[path]...)
}

func (s *VersionStore) GetVersion(path string, version uint64) (VersionRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, rec := range s.history[path] {
		if rec.Version == version {
			return rec, true
		}
	}
	return VersionRecord{}, false
}
