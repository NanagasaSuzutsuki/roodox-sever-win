package lock

import (
	"sync"
	"time"
)

type LockInfo struct {
	Owner    string
	ExpireAt time.Time
}

type Manager struct {
	mu         sync.Mutex
	locks      map[string]LockInfo // key = path
	defaultTTL time.Duration
}

func NewManager(defaultTTL time.Duration) *Manager {
	return &Manager{
		locks:      make(map[string]LockInfo),
		defaultTTL: defaultTTL,
	}
}

// Acquire 尝试加锁。返回：是否成功、当前持有者、到期时间。
func (m *Manager) Acquire(path, clientID string, ttl time.Duration) (ok bool, owner string, expireAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ttl <= 0 {
		ttl = m.defaultTTL
	}

	now := time.Now()
	info, exists := m.locks[path]

	// 没锁，或者锁已经过期 → 直接抢到
	if !exists || now.After(info.ExpireAt) {
		exp := now.Add(ttl)
		m.locks[path] = LockInfo{
			Owner:    clientID,
			ExpireAt: exp,
		}
		return true, clientID, exp
	}

	// 有锁且未过期 → 抢不到
	return false, info.Owner, info.ExpireAt
}

// Renew 续约，仅持有者才能续约
func (m *Manager) Renew(path, clientID string, ttl time.Duration) (ok bool, expireAt time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.locks[path]
	if !exists {
		return false, time.Time{}
	}
	if info.Owner != clientID {
		return false, info.ExpireAt
	}

	if ttl <= 0 {
		ttl = m.defaultTTL
	}

	now := time.Now()
	exp := now.Add(ttl)
	info.ExpireAt = exp
	m.locks[path] = info
	return true, exp
}

// Release 释放锁，仅持有者可以释放
func (m *Manager) Release(path, clientID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	info, exists := m.locks[path]
	if !exists {
		return false
	}
	if info.Owner != clientID {
		return false
	}
	delete(m.locks, path)
	return true
}

// CleanupExpired 可选：清理过期锁，防止 map 里一直有垃圾
func (m *Manager) CleanupExpired() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for p, info := range m.locks {
		if now.After(info.ExpireAt) {
			delete(m.locks, p)
		}
	}
}
