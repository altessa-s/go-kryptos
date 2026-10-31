// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"slices"
	"sync"

	"github.com/altessa-s/go-kryptos"
)

// Subscription owns buffered events until delivery. Recipients must Clear delivered events.
type Subscription[T any] struct {
	mu     sync.Mutex
	events chan *kryptos.Event[T]
	done   chan struct{}
	o      *watchOptions
	stop   func() bool
	remove func()
}

func (s *Subscription[T]) Events() <-chan *kryptos.Event[T] { return s.events }
func (s *Subscription[T]) Done() <-chan struct{}            { return s.done }
func (s *Subscription[T]) Close()                           { s.remove() }
func (s *Subscription[T]) close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return
	default:
	}
	if s.stop != nil {
		s.stop()
	}
	close(s.done)
	close(s.events)
	for e := range s.events {
		e.Clear()
	}
}
func (s *Subscription[T]) send(e *kryptos.Event[T]) {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		e.Clear()
		return
	default:
	}
	select {
	case s.events <- e:
		return
	default:
	}
	if s.o.overflow == DropOldest {
		select {
		case old := <-s.events:
			old.Clear()
		default:
		}
		select {
		case s.events <- e:
			return
		default:
		}
	}
	e.Clear()
}

// Watch subscribes to cycle and manager write events, optionally including cached initial values.
func (m *Manager[T]) Watch(ctx context.Context, opts ...WatchOption) (out *Subscription[T], err error) {
	defer recoverClone(&err)
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	o := newWatchOptions(opts...)
	if o.buffer < 1 || o.overflow < DropNewest || o.overflow > DropOldest {
		return nil, kryptos.ErrInvalidOptions
	}
	for _, k := range o.watchKeys {
		if e := kryptos.ValidateKey(k); e != nil {
			return nil, e
		}
	}
	s := &Subscription[T]{events: make(chan *kryptos.Event[T], o.buffer), done: make(chan struct{}), o: o}
	s.remove = func() { m.mu.Lock(); defer m.mu.Unlock(); delete(m.watchers, s); s.close() }
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return nil, kryptos.ErrClosed
	}
	transferred := false
	defer func() {
		if !transferred {
			delete(m.watchers, s)
			s.close()
		}
	}()
	m.watchers[s] = struct{}{}
	s.stop = context.AfterFunc(ctx, s.Close)
	if !o.skipInitial {
		for _, k := range m.cache.Keys() {
			v, _ := m.cache.Get(k)
			m.deliver(s, kryptos.Created, k, v)
		}
	}
	transferred = true
	return s, nil
}
func (m *Manager[T]) notify(kind kryptos.EventType, key string, v *kryptos.Secret[T]) {
	for s := range m.watchers {
		m.deliver(s, kind, key, v)
	}
}
func (m *Manager[T]) deliver(s *Subscription[T], kind kryptos.EventType, key string, v *kryptos.Secret[T]) {
	if len(s.o.watchKeys) > 0 && !slices.Contains(s.o.watchKeys, key) {
		return
	}
	if len(s.o.watchTypes) > 0 && !slices.Contains(s.o.watchTypes, kind) {
		return
	}
	s.send(&kryptos.Event[T]{Type: kind, Key: key, Secret: v.Clone(), Time: m.o.clock.Now()})
}
