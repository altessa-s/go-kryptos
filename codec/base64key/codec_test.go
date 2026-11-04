// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package base64key_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/base64key"
)

func FuzzRoundTrip(f *testing.F) {
	f.Add("secret")
	f.Fuzz(func(t *testing.T, value string) {
		c := base64key.New()
		encoded, e := c.Encode(value)
		require.NoError(t, e)
		decoded, e := c.Decode(encoded)
		require.NoError(t, e)
		require.Equal(t, value, decoded)
	})
}

func FuzzDecode(f *testing.F) {
	f.Add("c2VjcmV0")
	f.Add("!")
	f.Fuzz(func(t *testing.T, s string) {
		c := base64key.New()

		v, e := c.Decode(s)
		if e != nil {
			return
		}

		encoded, e := c.Encode(v)
		require.NoError(t, e)
		again, e := c.Decode(encoded)
		require.NoError(t, e)
		require.Equal(t, v, again)
	})
}
