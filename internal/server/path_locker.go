package server

import (
	"sort"
	"sync"
)

type PathLocker struct {
	locks sync.Map // map[path]*sync.Mutex
}

func NewPathLocker() *PathLocker {
	return &PathLocker{}
}

func (l *PathLocker) Lock(path string) func() {
	v, _ := l.locks.LoadOrStore(path, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func (l *PathLocker) LockMany(paths ...string) func() {
	if len(paths) == 0 {
		return func() {}
	}

	ordered := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)

	unlocks := make([]func(), 0, len(ordered))
	for _, path := range ordered {
		unlocks = append(unlocks, l.Lock(path))
	}

	return func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
}
