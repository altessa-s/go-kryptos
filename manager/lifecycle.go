// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"

	"github.com/altessa-s/go-kryptos"
)

func (m *Manager[T]) begin(ctx context.Context) (context.Context, func(), error) {
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil, nil, kryptos.ErrClosed
	}
	m.active.Add(1)
	m.mu.Unlock()
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(m.life, cancel)
	return work, func() { stop(); cancel(); m.active.Done() }, nil
}

// Close stops admission, cancels work, drains it, and closes the provider once.
// A canceled waiter may call Close again to wait for completion.
func (m *Manager[T]) Close(ctx context.Context) error {
	m.closeOnce.Do(func() {
		m.mu.Lock()
		m.closed = true
		m.cancel()
		m.group.Close()
		m.mu.Unlock()
		go m.finishClose(ctx)
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.done:
		return m.closeErr
	}
}
func (m *Manager[T]) finishClose(ctx context.Context) {
	m.active.Wait()
	m.group.Wait()
	m.clearOwned()
	closeCtx, cancel := context.WithTimeout(ctx, m.o.cycleTimeout)
	defer cancel()
	m.closeErr = m.p.Close(closeCtx)
	close(m.done)
}

// Purge clears all cached copies.
func (m *Manager[T]) Purge(ctx context.Context) error {
	_, end, e := m.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	m.reconcile.Lock()
	defer m.reconcile.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Purge()
	return nil
}

// Len returns the number of cached keys.
func (m *Manager[T]) Len() int { m.mu.Lock(); defer m.mu.Unlock(); return m.cache.Len() }

func (m *Manager[T]) clearOwned() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache.Purge()
	clear(m.versions)
	for s := range m.watchers {
		s.close()
	}
	clear(m.watchers)
}
