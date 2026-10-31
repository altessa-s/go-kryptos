// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"errors"
	"iter"
	"time"

	"github.com/altessa-s/go-atlas-core/runtime/concurrency"

	"github.com/altessa-s/go-kryptos"
)

// RunUpdateCycle reconciles provider state. Overlapping cycles return immediately.
func (m *Manager[T]) RunUpdateCycle(ctx context.Context) error {
	ctx, end, e := m.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	return m.guard.Run(ctx)
}
func (m *Manager[T]) update(ctx context.Context) (err error) {
	ctx, cancel := context.WithTimeout(ctx, m.o.cycleTimeout)
	defer cancel()
	start := m.o.clock.Now()
	defer func() { m.o.metrics.Cycled(m.o.clock.Now().Sub(start), err) }()
	m.mu.Lock()
	m.dirty = make(map[string]bool)
	m.mu.Unlock()
	defer func() { m.mu.Lock(); m.dirty = nil; m.mu.Unlock() }()
	values, err := m.listForCycle(ctx)
	defer func() {
		for _, s := range values {
			s.Clear()
		}
	}()
	if err != nil {
		return err
	}
	if err = m.validateListing(values); err != nil {
		return err
	}
	m.reconcile.Lock()
	defer m.reconcile.Unlock()
	return m.reconcileListing(values)
}
func (m *Manager[T]) reconcileListing(values []*kryptos.Secret[T]) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return kryptos.ErrClosed
	}
	if len(values) == 0 {
		m.empty++
		if m.empty < m.o.emptyListingThreshold {
			return nil
		}
	} else {
		m.empty = 0
	}
	seen := make(map[string]bool, len(values))
	for _, s := range values {
		seen[s.Key] = true
		if m.dirty[s.Key] {
			continue
		}
		clear(s.Encoded)
		s.Encoded = nil
		old, ok := m.versions[s.Key]
		if ok && old == s.Version {
			if _, cached := m.cache.Get(s.Key); !cached {
				m.cache.Put(s.Key, s.Clone())
			}
			continue
		}
		kind := kryptos.Created
		if ok {
			kind = kryptos.Updated
		}
		m.cache.Put(s.Key, s.Clone())
		m.versions[s.Key] = s.Version
		m.notify(kind, s.Key, s)
	}
	for key := range m.versions {
		if !seen[key] && !m.dirty[key] {
			m.cache.Remove(key)
			delete(m.versions, key)
			m.notify(kryptos.Deleted, key, nil)
		}
	}
	m.lastSync = m.o.clock.Now()
	m.o.metrics.CacheSize(m.cache.Len())
	return nil
}

// LastSync returns the last completed reconciliation time.
func (m *Manager[T]) LastSync() time.Time { m.mu.Lock(); defer m.mu.Unlock(); return m.lastSync }

// Warm loads keys, reporting every failure.
func (m *Manager[T]) Warm(ctx context.Context, keys ...string) error {
	var errs []error
	err := concurrency.Process(ctx, keys, func(work context.Context, k string) error { s, e := m.Get(work, k); s.Clear(); return e },
		concurrency.WithConcurrency[string](DefaultWarmConcurrency), concurrency.WithOnError(func(_ string, e error) { errs = append(errs, e) }))
	if len(errs) == 0 {
		return err
	}
	return errors.Join(errs...)
}

func (m *Manager[T]) listForCycle(ctx context.Context) (values []*kryptos.Secret[T], err error) {
	load := func(work context.Context) error { var e error; values, e = m.p.List(work); return e }
	if m.o.filterRebuilder != nil {
		err = m.o.filterRebuilder.Rebuild(ctx, func(work context.Context) iter.Seq2[string, error] {
			return func(yield func(string, error) bool) {
				if e := load(work); e != nil {
					yield("", e)
					return
				}
				for _, s := range values {
					if s != nil && !yield(s.Key, nil) {
						return
					}
				}
			}
		})
	} else {
		err = load(ctx)
	}
	return values, err
}
func (m *Manager[T]) validateListing(values []*kryptos.Secret[T]) error {
	for _, s := range values {
		if s == nil {
			return kryptos.ErrMalformed
		}
		if e := kryptos.ValidateKey(s.Key); e != nil {
			return e
		}
		if !m.o.allowShallowClone {
			if e := kryptos.CheckPayload(s.Value); e != nil {
				return e
			}
		}
	}
	return nil
}
