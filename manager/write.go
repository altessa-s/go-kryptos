// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"errors"

	"github.com/altessa-s/go-kryptos"
)

// Put writes an independent copy and refreshes the cache, including committed cleanup failures.
func (m *Manager[T]) Put(ctx context.Context, key string, value T) error {
	if e := kryptos.ValidateKey(key); e != nil {
		return e
	}
	if !m.o.allowShallowClone {
		if e := kryptos.CheckPayload(value); e != nil {
			return e
		}
	}
	ctx, end, e := m.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	m.reconcile.RLock()
	defer m.reconcile.RUnlock()
	unlock := m.keys.Lock(key)
	defer unlock()
	v := kryptos.NewSecret(key, value, nil, "")
	defer v.Clear()
	e = m.policy.Do(ctx, func(work context.Context) error {
		owned := v.Clone()
		defer owned.Clear()
		return m.p.Put(work, key, owned.Value)
	})
	_, committed := errors.AsType[*kryptos.CommittedError](e)
	if e != nil && !committed {
		return e
	}
	m.group.Forget(key)
	m.mu.Lock()
	_, existed := m.versions[key]
	m.cache.Remove(key)
	m.mu.Unlock()
	var filterErr error
	if m.o.keyFilter != nil {
		filterErr = m.o.keyFilter.Add(ctx, key)
	}
	fresh, readErr := m.fetch(ctx, key)
	if readErr != nil {
		fresh = v.Clone()
	}
	defer fresh.Clear()
	m.mu.Lock()
	if !m.closed {
		m.cache.Put(key, fresh.Clone())
		m.versions[key] = fresh.Version
		kind := kryptos.Created
		if existed {
			kind = kryptos.Updated
		}
		m.notify(kind, key, fresh)
	}
	m.mu.Unlock()
	return errors.Join(e, filterErr, readErr)
}

// Delete removes a backend value and every cached copy.
func (m *Manager[T]) Delete(ctx context.Context, key string) error {
	if e := kryptos.ValidateKey(key); e != nil {
		return e
	}
	ctx, end, e := m.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	m.reconcile.RLock()
	defer m.reconcile.RUnlock()
	unlock := m.keys.Lock(key)
	defer unlock()
	e = m.policy.Do(ctx, func(work context.Context) error { return m.p.Delete(work, key) })
	if e != nil && !errors.Is(e, kryptos.ErrNotFound) {
		return e
	}
	m.group.Forget(key)
	m.mu.Lock()
	m.cache.Remove(key)
	delete(m.versions, key)
	if e == nil {
		m.notify(kryptos.Deleted, key, nil)
	}
	m.mu.Unlock()
	if m.o.filterRebuilder == nil && m.o.filterDeleter != nil {
		e = errors.Join(e, m.o.filterDeleter.Delete(ctx, key))
	}
	return e
}
