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

func TestFetchCannotOverwritePut(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New(map[string]string{"key": "old"})
		require.NoError(t, e)
		entered := make(chan struct{})
		release := make(chan struct{})
		var once sync.Once
		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(ctx context.Context, k string) (*kryptos.Secret[string], error) {
			once.Do(func() { close(entered); <-release })
			return base.Get(ctx, k)
		}}
		m, e := manager.New[string](p)
		require.NoError(t, e)
		defer m.Close(t.Context())
		var wg sync.WaitGroup
		wg.Go(func() { s, e := m.Get(t.Context(), "key"); require.NoError(t, e); s.Clear() })
		<-entered
		wg.Go(func() { require.NoError(t, m.Put(t.Context(), "key", "new")) })
		close(release)
		wg.Wait()
		s, e := m.Get(t.Context(), "key")
		require.NoError(t, e)
		require.Equal(t, "new", s.Value)
		s.Clear()
	})
}
func TestLastWaiterCancelsWork(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New[string](nil)
		require.NoError(t, e)
		stopped := make(chan struct{})
		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(ctx context.Context, _ string) (*kryptos.Secret[string], error) {
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		}}
		m, e := manager.New[string](p)
		require.NoError(t, e)
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { _, e := m.Get(ctx, "key"); done <- e }()
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
		<-stopped
		require.NoError(t, m.Close(t.Context()))
	})
}
func TestCloseDeadlineAndDrain(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New[string](nil)
		require.NoError(t, e)
		release := make(chan struct{})
		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(context.Context, string) (*kryptos.Secret[string], error) {
			<-release
			return nil, kryptos.ErrNotFound
		}}
		m, e := manager.New[string](p)
		require.NoError(t, e)
		done := make(chan struct{})
		go func() { _, _ = m.Get(t.Context(), "key"); close(done) }()
		synctest.Wait()
		ctx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		require.ErrorIs(t, m.Close(ctx), context.DeadlineExceeded)
		close(release)
		<-done
		require.NoError(t, m.Close(t.Context()))
	})
}
func TestCycleNoOverlapAndMutation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New(map[string]string{"key": "old"})
		require.NoError(t, e)
		entered := make(chan struct{})
		release := make(chan struct{})
		p := &kryptostest.Provider[string]{Backend: base, ListFunc: func(ctx context.Context) ([]*kryptos.Secret[string], error) {
			close(entered)
			<-release
			return base.List(ctx)
		}}
		m, e := manager.New[string](p)
		require.NoError(t, e)
		defer m.Close(t.Context())
		var wg sync.WaitGroup
		wg.Go(func() { require.NoError(t, m.RunUpdateCycle(t.Context())) })
		<-entered
		require.NoError(t, m.RunUpdateCycle(t.Context()))
		wg.Go(func() { require.NoError(t, m.Put(t.Context(), "key", "new")) })
		close(release)
		wg.Wait()
		s, e := m.Get(t.Context(), "key")
		require.NoError(t, e)
		require.Equal(t, "new", s.Value)
		s.Clear()
	})
}
