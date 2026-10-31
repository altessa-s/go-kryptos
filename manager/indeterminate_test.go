// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

type indeterminateProvider struct {
	kryptos.Provider[[]byte]
	writes atomic.Int32
}

func (p *indeterminateProvider) PutResult(ctx context.Context, key string, value []byte) (kryptos.WriteResult, error) {
	p.writes.Add(1)
	result, err := p.Provider.PutResult(ctx, key, value)
	if err != nil {
		return result, err
	}
	return result, kryptos.ErrIndeterminate
}

func TestIndeterminateWriteBypassesNegativeFilter(t *testing.T) {
	t.Parallel()
	backend, err := memory.New(map[string][]byte{})
	require.NoError(t, err)
	p := &indeterminateProvider{Provider: backend}
	f := &filter{missing: true}
	m, err := manager.New[[]byte](p, manager.WithKeyFilter[[]byte](f))
	require.NoError(t, err)
	defer m.Close(t.Context())
	sub, err := m.Watch(t.Context(), manager.WithSkipInitial())
	require.NoError(t, err)
	defer sub.Close()
	for _, value := range []string{"first", "second"} {
		require.ErrorIs(t, m.Put(t.Context(), "key", []byte(value)), kryptos.ErrIndeterminate)
		cached, err := m.Cached("key")
		require.Nil(t, cached)
		require.ErrorIs(t, err, kryptos.ErrNotFound)
		got, err := m.Get(t.Context(), "key")
		require.NoError(t, err)
		require.Equal(t, []byte(value), got.Value)
		got.Clear()
		select {
		case event := <-sub.Events():
			event.Clear()
			t.Fatal("indeterminate write emitted a success event")
		default:
		}
	}
	require.EqualValues(t, 2, p.writes.Load())
	require.Zero(t, f.adds.Load())
}
