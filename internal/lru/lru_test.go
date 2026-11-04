// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lru_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/lru"
)

func TestEviction(t *testing.T) {
	t.Parallel()

	var evicted []int

	c, e := lru.New(2, 1, func(_ string, v int) { evicted = append(evicted, v) })
	require.NoError(t, e)
	c.Put("a", 1)
	c.Put("b", 2)
	v, ok := c.Get("a")
	require.True(t, ok)
	require.Equal(t, 1, v)
	c.Put("c", 3)
	require.Equal(t, []int{2}, evicted)
	c.Put("a", 4)
	require.Equal(t, []int{2, 1}, evicted)
	require.Len(t, c.Keys(), 2)
	require.Equal(t, 2, c.Len())
	c.Remove("c")
	c.Purge()
	require.Zero(t, c.Len())

	_, e = lru.New[int](0, 1, nil)
	require.Error(t, e)
	c, e = lru.New[int](3, 2, nil)
	require.NoError(t, e)
	c.Purge()
}
