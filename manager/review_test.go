// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

type addFailure struct{}

func (addFailure) MightExist(context.Context, string) (bool, error) { return true, nil }
func (addFailure) Add(context.Context, string) error                { return kryptos.ErrUnavailable }

func TestPostWriteFailuresAreCommitted(t *testing.T) {
	t.Parallel()
	for _, readFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "filter", true: "readback"}[readFails], func(t *testing.T) {
			t.Parallel()
			base, err := memory.New(map[string]string{"key": "old"})
			require.NoError(t, err)
			p := &kryptostest.Provider[string]{Backend: base}
			if readFails {
				p.GetFunc = func(context.Context, string) (*kryptos.Secret[string], error) { return nil, kryptos.ErrMalformed }
			}
			m, err := manager.New[string](p, manager.WithKeyFilter[string](addFailure{}))
			require.NoError(t, err)
			defer m.Close(t.Context())
			w, err := m.Watch(t.Context(), manager.WithSkipInitial())
			require.NoError(t, err)
			defer w.Close()
			err = m.Put(t.Context(), "key", "new")
			require.Error(t, err)
			require.IsType(t, &kryptos.CommittedError{}, err)
			got, err := m.Get(t.Context(), "key")
			require.NoError(t, err)
			require.Equal(t, "new", got.Value)
			require.Nil(t, got.Encoded)
			got.Clear()
			event := <-w.Events()
			require.Equal(t, kryptos.Updated, event.Type)
			event.Clear()
		})
	}
}

func TestTinyCacheConcurrentMisses(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		p, err := memory.New(map[string]string{"first": "one", "second": "two"})
		require.NoError(t, err)
		m, err := manager.New(p, manager.WithCacheSize[string](1))
		require.NoError(t, err)
		defer m.Close(t.Context())
		var wg sync.WaitGroup
		for range 50 {
			for _, key := range []string{"first", "second"} {
				wg.Go(func() {
					ctx, cancel := context.WithTimeout(t.Context(), time.Second)
					defer cancel()
					s, err := m.Get(ctx, key)
					require.NoError(t, err)
					require.NotEmpty(t, s.Value)
					s.Clear()
				})
			}
		}
		wg.Wait()
	})
}

func TestTerminalErrorsAreNotRetried(t *testing.T) {
	t.Parallel()
	for _, want := range []error{kryptos.ErrPermissionDenied, kryptos.ErrFailedPrecondition, kryptos.ErrMalformed, kryptos.ErrInvalidOptions, kryptos.ErrValueDecoding, kryptos.ErrValueEncoding, kryptos.ErrKeyEncoding, kryptos.ErrKeyDecoding} {
		t.Run(want.Error(), func(t *testing.T) {
			t.Parallel()
			base, err := memory.New[string](nil)
			require.NoError(t, err)
			calls := 0
			p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(context.Context, string) (*kryptos.Secret[string], error) { calls++; return nil, want }}
			m, err := manager.New[string](p)
			require.NoError(t, err)
			defer m.Close(t.Context())
			_, err = m.Get(t.Context(), "key")
			require.ErrorIs(t, err, want)
			require.Equal(t, 1, calls)
		})
	}
}

type slowClose struct {
	*memory.Store[string]
	done chan struct{}
}

func (p *slowClose) Close(ctx context.Context) error { <-ctx.Done(); close(p.done); return ctx.Err() }
func TestProviderCloseBoundedByCaller(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, err := memory.New[string](nil)
		require.NoError(t, err)
		p := &slowClose{Store: base, done: make(chan struct{})}
		m, err := manager.New[string](p)
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		require.ErrorIs(t, m.Close(ctx), context.DeadlineExceeded)
		<-p.done
		require.ErrorIs(t, m.Close(t.Context()), context.DeadlineExceeded)
	})
}
