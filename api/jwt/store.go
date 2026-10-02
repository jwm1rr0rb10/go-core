package jwt

import (
	"context"
	"sync"
	"time"
)

// RefreshStore records consumed refresh token ids so that each refresh token
// can be exchanged only once (rotation with reuse detection).
//
// Consume must be atomic: when two callers present the same id concurrently,
// exactly one may succeed. It returns ErrTokenRevoked (possibly wrapped) when
// the id was already consumed or revoked. expiresAt is the token expiry; the
// record may be dropped after it.
type RefreshStore interface {
	Consume(ctx context.Context, jti string, expiresAt time.Time) error
}

// MemoryRefreshStore is an in-process RefreshStore. It is suitable for a
// single instance and tests; replicas need a shared store.
type MemoryRefreshStore struct {
	mu        sync.Mutex
	used      map[string]time.Time
	now       func() time.Time
	nextSweep time.Time
}

// NewMemoryRefreshStore returns an empty MemoryRefreshStore.
func NewMemoryRefreshStore() *MemoryRefreshStore {
	return &MemoryRefreshStore{used: make(map[string]time.Time), now: time.Now}
}

// Consume implements RefreshStore. Expired records are swept lazily at most
// once a minute.
func (s *MemoryRefreshStore) Consume(_ context.Context, jti string, expiresAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if now.After(s.nextSweep) {
		for id, exp := range s.used {
			if now.After(exp) {
				delete(s.used, id)
			}
		}
		s.nextSweep = now.Add(time.Minute)
	}
	if _, ok := s.used[jti]; ok {
		return ErrTokenRevoked
	}
	s.used[jti] = expiresAt
	return nil
}

// Revoke marks jti as consumed until expiresAt, e.g. on logout.
func (s *MemoryRefreshStore) Revoke(jti string, expiresAt time.Time) {
	s.mu.Lock()
	s.used[jti] = expiresAt
	s.mu.Unlock()
}

// Len returns the number of tracked ids.
func (s *MemoryRefreshStore) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.used)
}
