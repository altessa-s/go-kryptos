// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"fmt"

	"github.com/altessa-s/go-kryptos"
)

// Cached returns a caller-owned copy without fetching.
func (m *Manager[T]) Cached(key string) (*kryptos.Secret[T], error) {
	if e := kryptos.ValidateKey(key); e != nil {
		return nil, e
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, kryptos.ErrClosed
	}
	v, ok := m.cache.Get(key)
	if !ok {
		return nil, kryptos.ErrNotFound
	}
	return v.Clone(), nil
}

// Get returns an independent payload copy.
func (m *Manager[T]) Get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	return m.get(ctx, key, false)
}

// GetShared returns a borrowed read-only value, invalidated by any cache mutation.
// The caller must externally serialize use against mutations; prefer Get.
func (m *Manager[T]) GetShared(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	return m.get(ctx, key, true)
}
func (m *Manager[T]) get(ctx context.Context, key string, shared bool) (*kryptos.Secret[T], error) {
	if e := kryptos.ValidateKey(key); e != nil {
		return nil, e
	}
	ctx, end, e := m.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	for {
		m.mu.Lock()
		v, ok := m.cache.Get(key)
		if ok {
			if !shared {
				v = v.Clone()
			}
			m.mu.Unlock()
			m.o.metrics.CacheHit()
			return v, nil
		}
		m.mu.Unlock()
		m.o.metrics.CacheMiss()
		if m.o.keyFilter != nil {
			exists, err := m.o.keyFilter.MightExist(ctx, key)
			if err != nil {
				m.o.logger.DebugContext(ctx, "secret filter failed", "key", key)
				m.o.metrics.Fetched(0, kryptos.ErrMalformed)
			} else if !exists {
				return nil, kryptos.ErrNotFound
			}
		}
		_, _, e = m.group.DoShared(ctx, key, func(work context.Context) (struct{}, error) { return m.fetchIntoCache(work, key) })
		if e != nil {
			return nil, e
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
	}
}
func (m *Manager[T]) fetch(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	start := m.o.clock.Now()
	var s *kryptos.Secret[T]
	err := m.policy.Do(ctx, func(work context.Context) error {
		var e error
		s, e = m.p.Get(work, key)
		if e != nil && s != nil {
			s.Clear()
			s = nil
		}
		return e
	})
	if err == nil {
		if s == nil {
			err = kryptos.ErrMalformed
		} else if !m.o.allowShallowClone {
			err = kryptos.CheckPayload(s.Value)
		}
	}
	if err != nil && s != nil {
		s.Clear()
		s = nil
	}
	m.o.metrics.Fetched(m.o.clock.Now().Sub(start), err)
	return s, err
}

func (m *Manager[T]) fetchIntoCache(work context.Context, key string) (_ struct{}, err error) {
	defer func() {
		if p := recover(); p != nil {
			m.o.logger.ErrorContext(work, "secret callback panicked", "key", key, "panic_type", fmt.Sprintf("%T", p))
			err = kryptos.ErrPanicked
		}
	}()
	m.reconcile.RLock()
	defer m.reconcile.RUnlock()
	unlock := m.keys.Lock(key)
	defer unlock()
	m.mu.Lock()
	_, ok := m.cache.Get(key)
	m.mu.Unlock()
	if ok {
		return struct{}{}, nil
	}
	s, err := m.fetch(work, key)
	if err != nil {
		return struct{}{}, err
	}
	defer s.Clear()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return struct{}{}, kryptos.ErrClosed
	}
	m.cache.Put(key, s.Clone())
	m.versions[key] = s.Version
	return struct{}{}, nil
}
