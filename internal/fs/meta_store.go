// internal/fs/meta_store.go
package fs

import "sync"

// Meta 是 Roodox 为每个路径维护的元信息
type Meta struct {
	Path      string
	IsDir     bool
	Size      int64
	MtimeUnix int64
	Hash      string
	Version   uint64
}

// MetaStore 目前用内存 map，将来可以换成 SQLite
type MetaStore struct {
	mu    sync.RWMutex
	items map[string]*Meta // key = path
}

func NewMetaStore() *MetaStore {
	return &MetaStore{
		items: make(map[string]*Meta),
	}
}

func (s *MetaStore) GetMeta(path string) (*Meta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.items[path]
	return m, ok
}

func (s *MetaStore) SetMeta(m *Meta) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[m.Path] = m
}

func (s *MetaStore) DeleteMeta(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, path)
}

// AllMetas 返回当前所有文件/目录的 meta 拷贝，用于 ListChangedFiles 等
func (s *MetaStore) AllMetas() []*Meta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Meta, 0, len(s.items))
	for _, m := range s.items {
		// 浅拷贝一份，避免外部修改内部指针
		cp := *m
		out = append(out, &cp)
	}
	return out
}
