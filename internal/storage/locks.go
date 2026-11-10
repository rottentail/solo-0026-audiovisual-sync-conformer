package storage

import "sync"

type keyLocks struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyLocks() *keyLocks {
	return &keyLocks{locks: make(map[string]*sync.Mutex)}
}

func (k *keyLocks) forKey(key string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	lock := k.locks[key]
	if lock == nil {
		lock = &sync.Mutex{}
		k.locks[key] = lock
	}
	return lock
}
