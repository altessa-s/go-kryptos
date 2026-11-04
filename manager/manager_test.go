// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if kryptostest.GoroutineLeaks() != nil {
		code = 1
	}

	os.Exit(code)
}
func newManager(t *testing.T, opts ...manager.Option[[]byte]) (*manager.Manager[[]byte], *memory.Store[[]byte]) {
	t.Helper()

	p, e := memory.New[[]byte](nil)
	require.NoError(t, e)
	m, e := manager.New(p, opts...)
	require.NoError(t, e)
	t.Cleanup(func() { require.NoError(t, m.Close(context.WithoutCancel(t.Context()))) })

	return m, p
}
func TestGetOwnershipAcrossInvalidation(t *testing.T) {
	t.Parallel()

	for _, action := range []string{"evict", "put", "delete", "purge", "close", "cycle"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			m, p := newManager(t, manager.WithCacheSize[[]byte](1))
			ctx := t.Context()
			require.NoError(t, m.Put(ctx, "key", []byte("old")))
			old, e := m.Get(ctx, "key")
			require.NoError(t, e)

			raw := old.Value

			switch action {
			case "evict":
				require.NoError(t, m.Put(ctx, "other", []byte("new")))
			case "put":
				require.NoError(t, m.Put(ctx, "key", []byte("new")))
			case "delete":
				require.NoError(t, m.Delete(ctx, "key"))
			case "purge":
				require.NoError(t, m.Purge(ctx))
			case "close":
				require.NoError(t, m.Close(ctx))
			case "cycle":
				require.NoError(t, p.Put(ctx, "key", []byte("new")))
				require.NoError(t, m.RunUpdateCycle(ctx))
			}

			require.Equal(t, []byte("old"), raw)
			old.Clear()
		})
	}
}
func TestCommittedWrite(t *testing.T) {
	t.Parallel()

	base, e := memory.New(map[string][]byte{"key": []byte("old")})
	require.NoError(t, e)

	var calls atomic.Int32

	p := &kryptostest.Provider[[]byte]{Backend: base, PutFunc: func(ctx context.Context, k string, v []byte) error {
		calls.Add(1)

		if e := base.Put(ctx, k, v); e != nil {
			return e
		}

		return &kryptos.CommittedError{Err: kryptos.ErrCleanupFailed}
	}}
	m, e := manager.New[[]byte](p)

	require.NoError(t, e)
	defer m.Close(t.Context())

	v, e := m.Get(t.Context(), "key")
	require.NoError(t, e)
	v.Clear()

	s, e := m.Watch(t.Context(), manager.WithSkipInitial())
	require.NoError(t, e)

	defer s.Close()

	require.ErrorIs(t, m.Put(t.Context(), "key", []byte("new")), kryptos.ErrCleanupFailed)
	require.EqualValues(t, 1, calls.Load())

	event := <-s.Events()
	require.Equal(t, kryptos.Updated, event.Type)
	event.Clear()

	v, e = m.Get(t.Context(), "key")
	require.NoError(t, e)
	require.Equal(t, []byte("new"), v.Value)
	v.Clear()
}
func TestPanickedCallbackIsSanitized(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New[string](nil)
		require.NoError(t, e)

		release := make(chan struct{})

		var calls atomic.Int32

		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(context.Context, string) (*kryptos.Secret[string], error) {
			calls.Add(1)
			<-release
			panic(kryptostest.Canary)
		}}

		var logs bytes.Buffer

		m, e := manager.New[string](p, manager.WithLogger[string](slog.New(slog.NewTextHandler(&logs, nil))))

		require.NoError(t, e)
		defer m.Close(t.Context())

		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_, err := m.Get(t.Context(), "key")
				require.ErrorIs(t, err, kryptos.ErrPanicked)
				require.NotContains(t, err.Error(), kryptostest.Canary)
			})
		}

		synctest.Wait()
		close(release)
		wg.Wait()
		require.EqualValues(t, 1, calls.Load())
		require.NotContains(t, logs.String(), kryptostest.Canary)
	})
}
func TestWarmAndLifecycle(t *testing.T) {
	t.Parallel()
	m, _ := newManager(t)
	require.ErrorIs(t, m.Warm(t.Context(), "missing", "other"), kryptos.ErrNotFound)
	require.NoError(t, m.Put(t.Context(), "key", []byte("value")))
	v, e := m.Cached("key")
	require.NoError(t, e)
	v.Clear()
	require.Equal(t, 1, m.Len())
	require.Equal(t, kryptos.Serving, m.Health(t.Context()))
	require.NoError(t, m.Close(t.Context()))
	_, e = m.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrClosed)
	require.ErrorIs(t, m.Put(t.Context(), "key", nil), kryptos.ErrClosed)
	require.Equal(t, kryptos.NotServing, m.Health(t.Context()))
}
func TestWatchOverflowAndCycle(t *testing.T) {
	t.Parallel()

	for _, policy := range []manager.OverflowPolicy{manager.DropNewest, manager.DropOldest} {
		t.Run(fmt.Sprint(policy), func(t *testing.T) {
			t.Parallel()
			m, p := newManager(t, manager.WithEmptyListingThreshold[[]byte](1))
			s, e := m.Watch(t.Context(), manager.WithBuffer(1), manager.WithOverflow(policy), manager.WithWatchKeys("key"))
			require.NoError(t, e)
			require.NoError(t, m.Put(t.Context(), "key", []byte("first")))
			require.NoError(t, m.Put(t.Context(), "key", []byte("second")))

			event := <-s.Events()

			want := "first"
			if policy == manager.DropOldest {
				want = "second"
			}

			require.Equal(t, want, string(event.Secret.Value))
			event.Clear()
			require.NoError(t, p.Delete(t.Context(), "key"))
			require.NoError(t, m.RunUpdateCycle(t.Context()))

			event = <-s.Events()
			require.Equal(t, kryptos.Deleted, event.Type)
			event.Clear()
			require.False(t, m.LastSync().IsZero())
			s.Close()
			s.Close()
			<-s.Done()
		})
	}
}
func TestCallerCancellation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New(map[string]string{"key": "value"})
		require.NoError(t, e)

		var calls atomic.Int32

		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(ctx context.Context, k string) (*kryptos.Secret[string], error) {
			calls.Add(1)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Second):
				return base.Get(ctx, k)
			}
		}}
		m, e := manager.New[string](p)

		require.NoError(t, e)
		defer m.Close(t.Context())

		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)

		go func() { _, e := m.Get(ctx, "key"); done <- e }()

		synctest.Wait()

		second := make(chan *kryptos.Secret[string], 1)

		go func() { v, e := m.Get(t.Context(), "key"); require.NoError(t, e); second <- v }()

		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)

		v := <-second
		require.Equal(t, "value", v.Value)
		v.Clear()
		require.EqualValues(t, 1, calls.Load())
	})
}
func TestResolver(t *testing.T) {
	t.Parallel()

	p, e := memory.New(map[string]string{"ns:key": "value"})
	require.NoError(t, e)

	r := manager.NewStringResolver(p)
	v, e := r.Resolve(t.Context(), "ns:key")
	require.NoError(t, e)
	require.Equal(t, "value", v)
	require.NoError(t, p.Close(t.Context()))
}
func ExampleNew() {
	p, _ := memory.New(map[string]string{"api-key": "example"})

	m, _ := manager.New(p)
	defer m.Close(context.Background())

	v, _ := m.Get(context.Background(), "api-key")
	defer v.Clear()

	fmt.Println(v) // Output: [REDACTED]
}
