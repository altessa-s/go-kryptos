// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clone_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/clone"
)

type cyclicNode struct {
	Next  *cyclicNode
	Bytes []byte
}

func TestCyclesAndDynamicValues(t *testing.T) {
	t.Parallel()

	n := &cyclicNode{Bytes: []byte("value")}
	n.Next = n
	c := clone.Copy(n)
	require.Same(t, c, c.Next)
	c.Bytes[0] = 'x'
	require.Equal(t, byte('v'), n.Bytes[0])
	require.NoError(t, clone.Check(n))
	require.Error(t, clone.Check(map[string]any{"x": func() {}}))

	m := map[string]any{"x": []byte("value")}
	copied := clone.Copy(m)
	copied["x"].([]byte)[0] = 'y'

	require.Equal(t, byte('v'), m["x"].([]byte)[0])
}
