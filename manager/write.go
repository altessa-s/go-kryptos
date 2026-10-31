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
	var result kryptos.WriteResult
	e = m.policy.Do(ctx, func(work context.Context) error {
		owned := v.Clone()
		defer owned.Clear()
		var err error
		result, err = m.p.PutResult(work, key, owned.Value)
		return err
	})
	_, committed := errors.AsType[*kryptos.CommittedError](e)
	if e != nil && !committed {
		if errors.Is(e, kryptos.ErrIndeterminate) {
			m.invalidateWrite(key)
		}
		return e
	}
	m.invalidateWrite(key)
	var filterErr error
	if m.o.keyFilter != nil {
		filterErr = m.o.keyFilter.Add(ctx, key)
		if filterErr != nil {
			m.o.metrics.FilterFailed()
		}
	}
	fresh, readErr := m.fetch(ctx, key)
	if readErr != nil {
		fresh = v.Clone()
	}
	defer fresh.Clear()
	clear(fresh.Encoded)
	fresh.Encoded = nil
	installErr := m.installWrite(key, fresh, result)
	if postErr := errors.Join(filterErr, readErr, installErr); postErr != nil {
		return &kryptos.CommittedError{Err: errors.Join(e, postErr)}
	}
	return e
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
	m.removeDeleted(key, e == nil)
	if m.o.filterRebuilder == nil && m.o.filterDeleter != nil {
		e = errors.Join(e, m.o.filterDeleter.Delete(ctx, key))
	}
	return e
}

// invalidateWrite also bypasses a possibly stale negative filter after an unknown outcome.
func (m *Manager[T]) invalidateWrite(key string) {
	m.group.Forget(key)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty != nil {
		m.dirty[key] = true
	}
	m.cache.Remove(key)
	m.versions[key] = ""
}

func (m *Manager[T]) installWrite(key string, fresh *kryptos.Secret[T], result kryptos.WriteResult) (err error) {
	defer recoverClone(&err)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.cache.Put(key, fresh.Clone())
		m.versions[key] = fresh.Version
		kind := kryptos.Created
		if !result.Created {
			kind = kryptos.Updated
		}
		m.notify(kind, key, fresh)
	}
	return nil
}

func (m *Manager[T]) removeDeleted(key string, notify bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.dirty != nil {
		m.dirty[key] = true
	}
	m.cache.Remove(key)
	delete(m.versions, key)
	if notify {
		m.notify(kryptos.Deleted, key, nil)
	}
}
