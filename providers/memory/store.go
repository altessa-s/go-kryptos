// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package memory

import (
	"context"
	"strconv"
	"sync"

	"github.com/altessa-s/go-kryptos"
)

// Store is a concurrent in-memory backend with independently owned results.
type Store[T any] struct {
	mu      sync.Mutex
	values  map[string]*kryptos.Secret[T]
	version uint64
	closed  bool
}

// New validates and copies initial values.
func New[T any](values map[string]T) (*Store[T], error) {
	s := &Store[T]{values: make(map[string]*kryptos.Secret[T])}
	for k, v := range values {
		if e := kryptos.ValidateKey(k); e != nil {
			return nil, e
		}
		if e := kryptos.CheckPayload(v); e != nil {
			return nil, e
		}
	}
	for k, v := range values {
		s.version++
		s.values[k] = kryptos.NewSecret(k, v, nil, strconv.FormatUint(s.version, 10))
	}
	return s, nil
}
func (*Store[T]) Name() string { return "memory" }
func (s *Store[T]) check(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.closed {
		return kryptos.ErrClosed
	}
	return nil
}
func (s *Store[T]) Get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	if e := kryptos.ValidateKey(key); e != nil {
		return nil, e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	v, ok := s.values[key]
	if !ok {
		return nil, kryptos.ErrNotFound
	}
	return v.Clone(), nil
}
func (s *Store[T]) List(ctx context.Context) ([]*kryptos.Secret[T], error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	out := make([]*kryptos.Secret[T], 0, len(s.values))
	for _, v := range s.values {
		out = append(out, v.Clone())
	}
	return out, nil
}
func (s *Store[T]) Put(ctx context.Context, key string, v T) error {
	if e := kryptos.ValidateKey(key); e != nil {
		return e
	}
	if e := kryptos.CheckPayload(v); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return e
	}
	s.version++
	old := s.values[key]
	s.values[key] = kryptos.NewSecret(key, v, nil, strconv.FormatUint(s.version, 10))
	old.Clear()
	return nil
}
func (s *Store[T]) Delete(ctx context.Context, key string) error {
	if e := kryptos.ValidateKey(key); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.check(ctx); e != nil {
		return e
	}
	v, ok := s.values[key]
	if !ok {
		return kryptos.ErrNotFound
	}
	delete(s.values, key)
	v.Clear()
	return nil
}
func (s *Store[T]) Ping(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.check(ctx)
}
func (s *Store[T]) Close(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for k, v := range s.values {
		v.Clear()
		delete(s.values, k)
	}
	return nil
}
