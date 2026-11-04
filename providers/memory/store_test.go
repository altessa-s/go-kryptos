// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package memory_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestContracts(t *testing.T) {
	t.Parallel()
	kryptostest.RunProviderContract(t, func(tb testing.TB) kryptos.Provider[[]byte] {
		tb.Helper()

		s, e := memory.New[[]byte](nil)
		require.NoError(tb, e)

		return s
	}, kryptostest.Caps{}, []byte("value"))
}
func TestConstructor(t *testing.T) {
	t.Parallel()

	_, e := memory.New(map[string]string{"a": "x"})
	require.ErrorIs(t, e, kryptos.ErrInvalidKey)
	_, e = memory.New(map[string]any{"key": make(chan int)})
	require.ErrorIs(t, e, kryptos.ErrUncloneablePayload)
	s, e := memory.New(map[string]string{"key": "value"})
	require.NoError(t, e)
	v, e := s.Get(t.Context(), "key")
	require.NoError(t, e)
	require.Equal(t, "value", v.Value)
	v.Clear()
	require.NoError(t, s.Close(t.Context()))
}
