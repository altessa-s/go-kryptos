// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptostest_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestContractSuite(t *testing.T) {
	t.Parallel()
	kryptostest.RunProviderContract(t, func(tb testing.TB) kryptos.Provider[string] {
		tb.Helper()
		p, e := memory.New[string](nil)
		require.NoError(tb, e)
		return p
	}, kryptostest.Caps{}, "value")
	kryptostest.RunCreationContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string], func(string) int) {
		tb.Helper()
		p, e := memory.New[string](nil)
		require.NoError(tb, e)
		return &kryptostest.Provider[string]{Backend: p}, &kryptostest.Provider[string]{Backend: p}, func(key string) int {
			value, e := p.Get(tb.Context(), key)
			require.NoError(tb, e)
			defer value.Clear()
			count, e := strconv.Atoi(value.Version)
			require.NoError(tb, e)
			return count
		}
	}, kryptostest.Caps{}, "value")
}
func TestFakeOverrides(t *testing.T) {
	t.Parallel()
	base, e := memory.New[string](nil)
	require.NoError(t, e)
	p := &kryptostest.Provider[string]{Backend: base, GetFunc: func(context.Context, string) (*kryptos.Secret[string], error) { return nil, kryptos.ErrNotFound }, PutFunc: func(context.Context, string, string) error { return kryptos.ErrConflict }, ListFunc: func(context.Context) ([]*kryptos.Secret[string], error) { return nil, kryptos.ErrUnavailable }}
	_, e = p.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrNotFound)
	require.ErrorIs(t, p.Put(t.Context(), "key", "x"), kryptos.ErrConflict)
	_, e = p.List(t.Context())
	require.ErrorIs(t, e, kryptos.ErrUnavailable)
	require.NoError(t, kryptostest.GoroutineLeaks())
	kryptostest.AssertRedacted(t, kryptos.NewSecret("key", "value", nil, "1"))
}

func TestManagerSuite(t *testing.T) {
	t.Parallel()
	kryptostest.RunManagerContract(t, func(tb testing.TB) kryptostest.Manager[string] {
		tb.Helper()
		p, e := memory.New[string](nil)
		require.NoError(tb, e)
		m, e := manager.New(p)
		require.NoError(tb, e)
		return m
	}, "value")
}
func TestPutDeleteSuite(t *testing.T) {
	t.Parallel()
	kryptostest.RunPutDeleteContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
		tb.Helper()
		p, e := memory.New[string](nil)
		require.NoError(tb, e)
		return &kryptostest.Provider[string]{Backend: p}, &kryptostest.Provider[string]{Backend: p}
	}, kryptostest.Caps{}, "value")
}
