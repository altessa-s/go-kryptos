// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptostest

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-core/runtime/leakcheck"

	kryptos "github.com/altessa-s/go-kryptos"
)

// Canary is a synthetic payload used to detect accidental disclosure.
const Canary = "kryptos-test-payload-canary"

// WritePolicy describes the backend's concurrent write semantics.
type WritePolicy int

const (
	// Unconditional backends let every concurrent write commit.
	Unconditional WritePolicy = iota
	// CheckAndSet backends reject a stale concurrent write with ErrConflict.
	CheckAndSet
)

// Caps describes backend capabilities for contract tests.
type Caps struct {
	WritePolicy WritePolicy
	ReadOnly    bool
}

// GoroutineLeaks checks for permanently blocked goroutines.
func GoroutineLeaks() error { return leakcheck.Check() }

// RunProviderContract verifies the shared backend contract with caller-supplied sample data.
func RunProviderContract[T any](t *testing.T, newProvider func(testing.TB) kryptos.Provider[T], caps Caps, values ...T) {
	t.Helper()
	t.Run("contract", func(t *testing.T) {
		t.Parallel()
		p := newProvider(t)
		ctx := t.Context()
		t.Cleanup(func() { require.NoError(t, p.Close(context.WithoutCancel(ctx))) })
		require.NotEmpty(t, p.Name())
		require.NoError(t, p.Ping(ctx))
		_, e := p.Get(ctx, "missing")
		require.ErrorIs(t, e, kryptos.ErrNotFound)

		var value T
		if len(values) > 0 {
			value = values[0]
		}

		for _, key := range []string{"", "a", "..", "a/b", string(make([]byte, 256))} {
			_, e = p.Get(ctx, key)
			require.ErrorIs(t, e, kryptos.ErrInvalidKey)
			require.ErrorIs(t, p.Put(ctx, key, value), kryptos.ErrInvalidKey)
			require.ErrorIs(t, p.Delete(ctx, key), kryptos.ErrInvalidKey)
		}

		canceled, cancel := context.WithCancel(ctx)
		cancel()
		require.ErrorIs(t, p.Ping(canceled), context.Canceled)
		_, e = p.Get(canceled, "key")
		require.ErrorIs(t, e, context.Canceled)
		_, e = p.List(canceled)
		require.ErrorIs(t, e, context.Canceled)
		require.ErrorIs(t, p.Put(canceled, "key", value), context.Canceled)
		require.ErrorIs(t, p.Delete(canceled, "key"), context.Canceled)

		if caps.ReadOnly {
			require.ErrorIs(t, p.Put(ctx, "key", value), kryptos.ErrReadOnly)
			return
		}

		input := kryptos.NewSecret("key", value, nil, "")
		result, e := p.PutResult(ctx, "key", input.Value)
		require.NoError(t, e)
		require.True(t, result.Created)
		mutateBytes(reflect.ValueOf(input.Value))
		input.Clear()

		first, e := p.Get(ctx, "key")
		require.NoError(t, e)
		require.Equal(t, value, first.Value)
		require.NotEmpty(t, first.Version)
		version := first.Version
		mutateBytes(reflect.ValueOf(first.Value))
		first.Clear()

		second, e := p.Get(ctx, "key")
		require.NoError(t, e)
		require.Equal(t, value, second.Value)
		second.Clear()

		result, e = p.PutResult(ctx, "key", value)
		require.NoError(t, e)
		require.False(t, result.Created)

		third, e := p.Get(ctx, "key")
		require.NoError(t, e)
		require.NotEqual(t, version, third.Version)
		third.Clear()

		all, e := p.List(ctx)
		require.NoError(t, e)
		require.Len(t, all, 1)

		for _, v := range all {
			v.Clear()
		}

		require.NoError(t, p.Delete(ctx, "key"))
		require.ErrorIs(t, p.Delete(ctx, "key"), kryptos.ErrNotFound)
		require.NoError(t, p.Close(canceled))
		require.NoError(t, p.Close(canceled))
		_, e = p.Get(ctx, "key")
		require.ErrorIs(t, e, kryptos.ErrClosed)
		require.ErrorIs(t, p.Ping(ctx), kryptos.ErrClosed)
		_, e = p.List(ctx)
		require.ErrorIs(t, e, kryptos.ErrClosed)
		require.ErrorIs(t, p.Put(ctx, "key", value), kryptos.ErrClosed)
		require.ErrorIs(t, p.Delete(ctx, "key"), kryptos.ErrClosed)
	})
}

// RunCreationContract uses two independent providers sharing one backend.
func RunCreationContract[T any](t *testing.T, pair func(testing.TB) (kryptos.Provider[T], kryptos.Provider[T], func(string) int), caps Caps, value T) {
	t.Helper()
	t.Run("independent_creation", func(t *testing.T) {
		t.Parallel()
		a, b, versions := pair(t)

		ctx := t.Context()
		defer func() { require.NoError(t, a.Close(context.WithoutCancel(ctx))) }()
		defer func() { require.NoError(t, b.Close(context.WithoutCancel(ctx))) }()

		start := make(chan struct{})

		type outcome struct {
			provider kryptos.Provider[T]
			err      error
		}

		results := make(chan outcome, 2)

		var wg sync.WaitGroup
		for _, p := range []kryptos.Provider[T]{a, b} {
			wg.Go(func() { <-start; results <- outcome{p, p.Put(ctx, "concurrent", value)} })
		}

		close(start)
		wg.Wait()
		close(results)

		conflicts := 0

		var loser kryptos.Provider[T]

		for result := range results {
			e := result.err
			if e != nil {
				require.Equal(t, CheckAndSet, caps.WritePolicy)
				require.ErrorIs(t, e, kryptos.ErrConflict)

				conflicts++
				loser = result.provider
			}
		}

		require.LessOrEqual(t, conflicts, 1)

		if conflicts > 0 {
			require.Equal(t, 1, versions("concurrent"))
			require.NoError(t, loser.Put(ctx, "concurrent", value))
		}

		const committedVersions = 2
		require.Equal(t, committedVersions, versions("concurrent"))

		all, e := a.List(ctx)
		require.NoError(t, e)
		require.Len(t, all, 1)

		for _, v := range all {
			v.Clear()
		}
	})
}

// Provider is a fault-injecting decorator; configure callbacks before concurrent use.
type Provider[T any] struct {
	Backend  kryptos.Provider[T]
	GetFunc  func(context.Context, string) (*kryptos.Secret[T], error)
	PutFunc  func(context.Context, string, T) error
	ListFunc func(context.Context) ([]*kryptos.Secret[T], error)
}

// Name returns "fake".
func (p *Provider[T]) Name() string { return "fake" }

// Get calls GetFunc when set, otherwise the backend.
func (p *Provider[T]) Get(c context.Context, k string) (*kryptos.Secret[T], error) {
	if p.GetFunc != nil {
		return p.GetFunc(c, k)
	}

	return p.Backend.Get(c, k)
}

// Put calls PutFunc when set, otherwise the backend.
func (p *Provider[T]) Put(c context.Context, k string, v T) error {
	if p.PutFunc != nil {
		return p.PutFunc(c, k, v)
	}

	return p.Backend.Put(c, k, v)
}

// PutResult calls PutFunc when set, reporting Created when the backend held
// no value; otherwise it calls the backend.
func (p *Provider[T]) PutResult(c context.Context, k string, v T) (kryptos.WriteResult, error) {
	if p.PutFunc == nil {
		return p.Backend.PutResult(c, k, v)
	}

	old, err := p.Backend.Get(c, k)
	old.Clear()

	return kryptos.WriteResult{Created: errors.Is(err, kryptos.ErrNotFound)}, p.PutFunc(c, k, v)
}

// List calls ListFunc when set, otherwise the backend.
func (p *Provider[T]) List(c context.Context) ([]*kryptos.Secret[T], error) {
	if p.ListFunc != nil {
		return p.ListFunc(c)
	}

	return p.Backend.List(c)
}

// Delete forwards to the backend.
func (p *Provider[T]) Delete(c context.Context, k string) error { return p.Backend.Delete(c, k) }

// Ping forwards to the backend.
func (p *Provider[T]) Ping(c context.Context) error { return p.Backend.Ping(c) }

// Close forwards to the backend.
func (p *Provider[T]) Close(c context.Context) error { return p.Backend.Close(c) }

// AssertRedacted checks formatted values without printing a failed secret.
func AssertRedacted(t testing.TB, v any) {
	t.Helper()

	for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		require.NotContains(t, fmt.Sprintf(format, v), Canary)
	}
}

// Manager is the structural manager surface exercised by RunManagerContract.
type Manager[T any] interface {
	kryptos.Getter[T]
	kryptos.Writer[T]
	Warm(context.Context, ...string) error
	Purge(context.Context) error
	Len() int
	RunUpdateCycle(context.Context) error
	Close(context.Context) error
}

// RunManagerContract verifies manager behavior over any provider composition.
func RunManagerContract[T any](t *testing.T, newManager func(testing.TB) Manager[T], value T) {
	t.Helper()
	t.Run("manager_contract", func(t *testing.T) {
		t.Parallel()
		m := newManager(t)

		ctx := t.Context()
		defer func() { require.NoError(t, m.Close(context.WithoutCancel(ctx))) }()

		require.NoError(t, m.Put(ctx, "key", value))
		require.NoError(t, m.Purge(ctx))
		require.NoError(t, m.Warm(ctx, "key"))
		s, e := m.Get(ctx, "key")
		require.NoError(t, e)
		require.Equal(t, value, s.Value)
		s.Clear()
		require.NoError(t, m.RunUpdateCycle(ctx))
		require.Positive(t, m.Len())
		require.NoError(t, m.Delete(ctx, "key"))
		_, e = m.Get(ctx, "key")
		require.ErrorIs(t, e, kryptos.ErrNotFound)
	})
}

// RunPutDeleteContract checks independent writers against one backend.
// CAS writes may conflict if deletion invalidates their observed version.
func RunPutDeleteContract[T any](t *testing.T, pair func(testing.TB) (kryptos.Provider[T], kryptos.Provider[T]), caps Caps, value T) {
	t.Helper()
	t.Run("concurrent_put_delete", func(t *testing.T) {
		t.Parallel()
		a, b := pair(t)

		ctx := t.Context()
		defer func() {
			require.NoError(t, a.Close(context.WithoutCancel(ctx)))
			require.NoError(t, b.Close(context.WithoutCancel(ctx)))
		}()

		require.NoError(t, a.Put(ctx, "racing", value))

		start := make(chan struct{})
		results := make(chan error, 2)

		go func() { <-start; results <- a.Put(ctx, "racing", value) }()
		go func() { <-start; results <- b.Delete(ctx, "racing") }()

		close(start)

		for range 2 {
			e := <-results
			if e == nil || errors.Is(e, kryptos.ErrNotFound) {
				continue
			}

			require.Equal(t, CheckAndSet, caps.WritePolicy)
			require.ErrorIs(t, e, kryptos.ErrConflict)
		}

		s, e := a.Get(ctx, "racing")
		if e != nil {
			require.ErrorIs(t, e, kryptos.ErrNotFound)
		} else {
			require.Equal(t, value, s.Value)
			s.Clear()
		}
	})
}

// mutateBytes exercises writable byte leaves in contract sample slices and structs.
func mutateBytes(v reflect.Value) {
	const byteMask = 0xff

	if !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.Slice:
		if v.Type().Elem().Kind() == reflect.Uint8 {
			for i := range v.Len() {
				if v.Index(i).CanSet() {
					v.Index(i).SetUint(v.Index(i).Uint() ^ byteMask)
				}
			}
		}
	case reflect.Struct:
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				mutateBytes(v.Field(i))
			}
		}
	default:
		// Scalars are immutable; custom payload ownership belongs in provider-specific tests.
	}
}
