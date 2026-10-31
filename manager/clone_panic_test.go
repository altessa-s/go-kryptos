// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

type panicClone struct{ remaining *atomic.Int32 }

func (p panicClone) Clone() panicClone {
	if p.remaining.Add(-1) == 0 {
		panic(kryptostest.Canary)
	}
	return p
}

func TestCachedClonePanicReleasesLock(t *testing.T) {
	t.Parallel()
	for _, cached := range []bool{false, true} {
		t.Run(map[bool]string{false: "Get", true: "Cached"}[cached], func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				count := &atomic.Int32{}
				p, err := memory.New(map[string]panicClone{"key": {remaining: count}})
				require.NoError(t, err)
				m, err := manager.New(p)
				require.NoError(t, err)
				first, err := m.Get(t.Context(), "key")
				require.NoError(t, err)
				first.Clear()
				count.Store(1)
				var got *kryptos.Secret[panicClone]
				if cached {
					got, err = m.Cached("key")
				} else {
					got, err = m.Get(t.Context(), "key")
				}
				require.Nil(t, got)
				require.ErrorIs(t, err, kryptos.ErrPanicked)
				require.NotContains(t, err.Error(), kryptostest.Canary)
				got, err = m.Get(t.Context(), "key")
				require.NoError(t, err)
				got.Clear()
				require.NoError(t, m.Put(t.Context(), "other", panicClone{remaining: count}))
				require.NoError(t, m.Close(t.Context()))
			})
		})
	}
}

// Return a payload that fails only during installation or watch delivery, after
// the provider read and write have completed.
type installPanicProvider struct {
	kryptos.Provider[panicClone]
	count  *atomic.Int32
	clones int32
}

func (p installPanicProvider) Get(ctx context.Context, key string) (*kryptos.Secret[panicClone], error) {
	s, err := p.Provider.Get(ctx, key)
	p.count.Store(p.clones)
	return s, err
}

func TestPutClonePanicReleasesLock(t *testing.T) {
	t.Parallel()
	for _, clones := range []int32{1, 2} {
		t.Run(map[int32]string{1: "installation", 2: "notification"}[clones], func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				count := &atomic.Int32{}
				p, err := memory.New(map[string]panicClone{})
				require.NoError(t, err)
				m, err := manager.New[panicClone](installPanicProvider{Provider: p, count: count, clones: clones})
				require.NoError(t, err)
				sub, err := m.Watch(t.Context(), manager.WithSkipInitial())
				require.NoError(t, err)
				err = m.Put(t.Context(), "key", panicClone{remaining: count})
				require.ErrorIs(t, err, kryptos.ErrPanicked)
				_, committed := errors.AsType[*kryptos.CommittedError](err)
				require.True(t, committed)
				require.NotContains(t, err.Error(), kryptostest.Canary)
				require.NoError(t, m.Purge(t.Context()))
				sub.Close()
				require.NoError(t, m.Close(t.Context()))
			})
		})
	}
}
