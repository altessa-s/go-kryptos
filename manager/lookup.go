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
func (m *Manager[T]) Cached(key string) (out *kryptos.Secret[T], err error) {
	defer recoverClone(&err)

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
	return m.get(ctx, key)
}

func (m *Manager[T]) get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	if e := kryptos.ValidateKey(key); e != nil {
		return nil, e
	}

	ctx, end, e := m.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()

	for attempt := 0; ; attempt++ {
		v, known, err := m.cachedCopy(key)
		if err != nil {
			return nil, err
		}

		if v != nil {
			m.o.metrics.CacheHit()
			return v, nil
		}

		if attempt == maxCacheAttempts {
			return m.fetchOwned(ctx, key)
		}

		m.o.metrics.CacheMiss()

		if m.o.keyFilter != nil {
			exists, err := m.o.keyFilter.MightExist(ctx, key)
			if err != nil {
				m.o.logger.DebugContext(ctx, "secret filter failed", "key", key)
				m.o.metrics.FilterFailed()
			} else if !exists && !known {
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

	clear(s.Encoded)
	s.Encoded = nil
	m.cache.Put(key, s.Clone())

	m.versions[key] = s.Version
	if m.dirty != nil {
		m.dirty[key] = true
	}

	return struct{}{}, nil
}

// fetchOwned bounds cache-contention retries without handing a cache-owned value to a waiter.
func (m *Manager[T]) fetchOwned(ctx context.Context, key string) (out *kryptos.Secret[T], err error) {
	defer func() {
		if recover() != nil {
			out.Clear()
			out = nil
			err = kryptos.ErrPanicked
		}
	}()

	m.reconcile.RLock()
	defer m.reconcile.RUnlock()

	unlock := m.keys.Lock(key)
	defer unlock()

	s, err := m.fetch(ctx, key)
	if err != nil {
		return nil, err
	}
	defer s.Clear()

	clear(s.Encoded)
	s.Encoded = nil

	return s.Clone(), nil
}

const maxCacheAttempts = 3

// cachedCopy keeps cloning synchronized with eviction and clearing.
func (m *Manager[T]) cachedCopy(key string) (out *kryptos.Secret[T], known bool, err error) {
	defer recoverClone(&err)

	m.mu.Lock()
	defer m.mu.Unlock()

	if v, ok := m.cache.Get(key); ok {
		return v.Clone(), true, nil
	}

	_, known = m.versions[key]

	return nil, known, nil
}

func recoverClone(err *error) {
	if recover() != nil {
		*err = kryptos.ErrPanicked
	}
}
