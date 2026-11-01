// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"context"
	"errors"
	"io"
	"iter"
	"log/slog"
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

type filter struct {
	fail                    bool
	missing                 bool
	adds, deletes, rebuilds atomic.Int32
}

func (f *filter) MightExist(context.Context, string) (bool, error) {
	if f.fail {
		return false, kryptos.ErrUnavailable
	}
	return !f.missing, nil
}
func (f *filter) Add(context.Context, string) error    { f.adds.Add(1); return nil }
func (f *filter) Delete(context.Context, string) error { f.deletes.Add(1); return nil }
func (f *filter) Rebuild(ctx context.Context, load func(context.Context) iter.Seq2[string, error]) error {
	f.rebuilds.Add(1)
	for _, e := range load(ctx) {
		if e != nil {
			return e
		}
	}
	return nil
}

type metrics struct{ hits, misses, fetched, cycled, sizes, filterFailed atomic.Int32 }

func (m *metrics) CacheHit()                    { m.hits.Add(1) }
func (m *metrics) CacheMiss()                   { m.misses.Add(1) }
func (m *metrics) Fetched(time.Duration, error) { m.fetched.Add(1) }
func (m *metrics) Cycled(time.Duration, error)  { m.cycled.Add(1) }
func (m *metrics) Skipped()                     {}
func (m *metrics) FilterFailed()                { m.filterFailed.Add(1) }
func (m *metrics) CacheSize(int)                { m.sizes.Add(1) }
func TestFiltersMetricsAndHealth(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		p, e := memory.New(map[string]string{"key": "value"})
		require.NoError(t, e)
		f := &filter{fail: true}
		stats := &metrics{}
		m, e := manager.New(p, manager.WithKeyFilter[string](f), manager.WithFilterRebuilder[string](f), manager.WithFilterDeleter[string](f), manager.WithMetrics[string](stats), manager.WithClock[string](kryptos.RealClock{}), manager.WithLogger[string](slog.New(slog.NewTextHandler(io.Discard, nil))), manager.WithStaleAfter[string](time.Minute), manager.WithCycleTimeout[string](time.Second), manager.WithCacheShards[string](2), manager.WithCacheSize[string](4))
		require.NoError(t, e)
		defer m.Close(t.Context())
		v, e := m.Get(t.Context(), "key")
		require.NoError(t, e)
		v.Clear()
		require.NoError(t, m.RunUpdateCycle(t.Context()))
		time.Sleep(2 * time.Minute)
		require.Equal(t, kryptos.Degraded, m.Health(t.Context()))
		require.NoError(t, m.Put(t.Context(), "key", "updated"))
		require.NoError(t, m.Delete(t.Context(), "key"))
		require.EqualValues(t, 1, f.adds.Load())
		require.Zero(t, f.deletes.Load())
		require.EqualValues(t, 1, f.rebuilds.Load())
		require.EqualValues(t, 1, stats.filterFailed.Load())
		require.Positive(t, stats.fetched.Load())
		require.Positive(t, stats.sizes.Load())
	})
}
func TestFilterMissingAndDelete(t *testing.T) {
	t.Parallel()
	p, e := memory.New[string](nil)
	require.NoError(t, e)
	f := &filter{missing: true}
	m, e := manager.New(p, manager.WithKeyFilter[string](f), manager.WithFilterDeleter[string](f))
	require.NoError(t, e)
	defer m.Close(t.Context())
	_, e = m.Get(t.Context(), "missing")
	require.ErrorIs(t, e, kryptos.ErrNotFound)
	require.NoError(t, m.Put(t.Context(), "key", "value"))
	require.NoError(t, m.Delete(t.Context(), "key"))
	require.EqualValues(t, 1, f.deletes.Load())
}
func TestRetryAndCAS(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		base, e := memory.New(map[string]string{"key": "value"})
		require.NoError(t, e)
		var gets, puts atomic.Int32
		p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(ctx context.Context, key string) (*kryptos.Secret[string], error) {
			if gets.Add(1) == 1 {
				return nil, kryptos.ErrUnavailable
			}
			return base.Get(ctx, key)
		}, PutFunc: func(context.Context, string, string) error { puts.Add(1); return kryptos.ErrConflict }}
		m, e := manager.New[string](p, manager.WithMaxRetries[string](2), manager.WithBackoff[string](func(int, error) time.Duration { return time.Second }))
		require.NoError(t, e)
		defer m.Close(t.Context())
		v, e := m.Get(t.Context(), "key")
		require.NoError(t, e)
		v.Clear()
		require.EqualValues(t, 2, gets.Load())
		require.ErrorIs(t, m.Put(t.Context(), "key", "new"), kryptos.ErrConflict)
		require.EqualValues(t, 1, puts.Load())
	})
}
func TestConstructorValidation(t *testing.T) {
	t.Parallel()
	p, e := memory.New[string](nil)
	require.NoError(t, e)
	for _, o := range []manager.Option[string]{manager.WithCacheSize[string](0), manager.WithCacheShards[string](0), manager.WithCacheShards[string](2000), manager.WithMaxRetries[string](-1), manager.WithEmptyListingThreshold[string](0), manager.WithCycleTimeout[string](0), manager.WithStaleAfter[string](0)} {
		_, e := manager.New(p, o)
		require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	}
	_, e = manager.New[string](nil)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	var typed *memory.Store[string]
	_, e = manager.New[string](typed)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	unsafe, e := memory.New[func()](nil)
	require.NoError(t, e)
	_, e = manager.New(unsafe)
	require.ErrorIs(t, e, kryptos.ErrUncloneablePayload)
	m, e := manager.New(unsafe, manager.WithAllowShallowClone[func()]())
	require.NoError(t, e)
	require.NoError(t, m.Close(t.Context()))
}
func TestWatchValidationAndCancellation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		m, _ := newManager(t)
		for _, opts := range [][]manager.WatchOption{{manager.WithBuffer(0)}, {manager.WithOverflow(99)}, {manager.WithWatchKeys("bad/key")}} {
			_, e := m.Watch(t.Context(), opts...)
			require.Error(t, e)
		}
		require.NoError(t, m.Put(t.Context(), "key", []byte("initial")))
		ctx, cancel := context.WithCancel(t.Context())
		s, e := m.Watch(ctx, manager.WithWatchKeys("key", "other"), manager.WithWatchTypes(kryptos.Created, kryptos.Updated))
		require.NoError(t, e)
		event := <-s.Events()
		require.Equal(t, kryptos.Created, event.Type)
		event.Clear()
		require.NoError(t, m.Put(t.Context(), "excluded", []byte("skip")))
		require.NoError(t, m.Delete(t.Context(), "key"))
		cancel()
		synctest.Wait()
		<-s.Done()
		require.NoError(t, m.Close(t.Context()))
		_, e = m.Watch(t.Context())
		require.ErrorIs(t, e, kryptos.ErrClosed)
	})
}
func TestUpdateFailuresAndEmptyThreshold(t *testing.T) {
	t.Parallel()
	base, e := memory.New(map[string]string{"key": "value"})
	require.NoError(t, e)
	m, e := manager.New(base, manager.WithEmptyListingThreshold[string](2))
	require.NoError(t, e)
	defer m.Close(t.Context())
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.NoError(t, base.Delete(t.Context(), "key"))
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Equal(t, 1, m.Len())
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Zero(t, m.Len())
}
func TestFetchMalformedAndReadBackRetry(t *testing.T) {
	t.Parallel()
	base, e := memory.New[string](nil)
	require.NoError(t, e)
	p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(context.Context, string) (*kryptos.Secret[string], error) { return nil, nil }}
	m, e := manager.New[string](p)
	require.NoError(t, e)
	defer m.Close(t.Context())
	_, e = m.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrMalformed)
	require.ErrorIs(t, m.Put(t.Context(), "key", "fallback"), kryptos.ErrMalformed)
	v, e := m.Cached("key")
	require.NoError(t, e)
	require.Equal(t, "fallback", v.Value)
	v.Clear()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.True(t, errors.Is(m.Purge(ctx), context.Canceled))
	_, e = m.Cached("missing")
	require.ErrorIs(t, e, kryptos.ErrNotFound)
	_, e = m.Cached("a")
	require.ErrorIs(t, e, kryptos.ErrInvalidKey)
}
func TestUpdateCycleRetriesListing(t *testing.T) {
	t.Parallel()
	base, e := memory.New(map[string]string{"key": "value"})
	require.NoError(t, e)
	var calls atomic.Int32
	p := &kryptostest.Provider[string]{Backend: base, ListFunc: func(ctx context.Context) ([]*kryptos.Secret[string], error) {
		if calls.Add(1) == 1 {
			return nil, kryptos.ErrUnavailable
		}
		return base.List(ctx)
	}}
	m, e := manager.New[string](p, manager.WithMaxRetries[string](2), manager.WithBackoff[string](func(int, error) time.Duration { return time.Millisecond }))
	require.NoError(t, e)
	defer m.Close(t.Context())
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Equal(t, int32(2), calls.Load())
	require.Equal(t, 1, m.Len())
}
func TestEmptyThresholdResetsOnFailedListing(t *testing.T) {
	t.Parallel()
	base, e := memory.New(map[string]string{"key": "value"})
	require.NoError(t, e)
	var fail atomic.Bool
	p := &kryptostest.Provider[string]{Backend: base, ListFunc: func(ctx context.Context) ([]*kryptos.Secret[string], error) {
		if fail.Load() {
			return nil, kryptos.ErrPermissionDenied
		}
		return base.List(ctx)
	}}
	m, e := manager.New[string](p, manager.WithEmptyListingThreshold[string](2))
	require.NoError(t, e)
	defer m.Close(t.Context())
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.NoError(t, base.Delete(t.Context(), "key"))
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	fail.Store(true)
	require.ErrorIs(t, m.RunUpdateCycle(t.Context()), kryptos.ErrPermissionDenied)
	fail.Store(false)
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Equal(t, 1, m.Len())
	require.NoError(t, m.RunUpdateCycle(t.Context()))
	require.Zero(t, m.Len())
}
