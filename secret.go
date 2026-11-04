// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos

import (
	"fmt"
	"io"
	"log/slog"
	"slices"
	"sync"

	"github.com/altessa-s/go-kryptos/internal/clone"
	"github.com/altessa-s/go-kryptos/internal/zero"
)

// Secret owns its payload. Do not copy the struct or mutate it concurrently.
type Secret[T any] struct {
	Key        string `json:"key"`
	EncodedKey string `json:"encoded_key"`
	Version    string `json:"version"`
	Value      T      `json:"-"`
	Encoded    []byte `json:"-"`
	mu         sync.Mutex
}

// NewSecret copies a payload and its encoded representation.
func NewSecret[T any](key string, v T, encoded []byte, version string) *Secret[T] {
	return &Secret[T]{Key: key, Value: clone.Copy(v), Encoded: slices.Clone(encoded), Version: version}
}

// Clone returns an independent copy.
func (s *Secret[T]) Clone() *Secret[T] {
	if s == nil {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	out := NewSecret(s.Key, s.Value, s.Encoded, s.Version)
	out.EncodedKey = s.EncodedKey

	return out
}

// Clear is idempotent and overwrites owned byte buffers.
func (s *Secret[T]) Clear() {
	if s == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	zero.Clear(&s.Value)
	clear(s.Encoded)
	s.Encoded = nil
}

// String redacts the complete secret.
func (s *Secret[T]) String() string { return "[REDACTED]" }

// GoString redacts Go-syntax formatting.
func (s *Secret[T]) GoString() string { return "[REDACTED]" }

// Format redacts every formatting verb.
func (s *Secret[T]) Format(f fmt.State, _ rune) { _, _ = io.WriteString(f, "[REDACTED]") }

// LogValue redacts structured logs.
func (s *Secret[T]) LogValue() slog.Value { return slog.StringValue("[REDACTED]") }

// CheckPayload rejects values whose mutable state cannot be copied safely.
func CheckPayload[T any](v T) error {
	if clone.Check(v) != nil {
		return ErrUncloneablePayload
	}

	return nil
}
