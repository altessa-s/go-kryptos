// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package zero_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/zero"
)

type custom struct{ data []byte }

func (c *custom) Zeroize() { clear(c.data) }
func TestClear(t *testing.T) {
	t.Parallel()
	b := []byte("secret")
	alias := b
	zero.Clear(&b)
	require.Nil(t, b)
	require.Equal(t, make([]byte, 6), alias)
	c := &custom{data: []byte("value")}
	bytes := c.data
	zero.Clear(&c)
	require.Nil(t, c)
	zero.Clear(&c)
	require.Equal(t, make([]byte, 5), bytes)
	s := "immutable"
	zero.Clear(&s)
	require.Empty(t, s)
}
