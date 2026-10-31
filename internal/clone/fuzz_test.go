// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clone_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/clone"
)

func FuzzCopy(f *testing.F) {
	f.Add([]byte("payload"))
	f.Fuzz(func(t *testing.T, b []byte) {
		c := clone.Copy(b)
		require.Equal(t, b, c)
		if len(c) > 0 {
			before := b[0]
			c[0] ^= 255
			require.Equal(t, before, b[0])
		}
	})
}
