// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package jsonvalue_test

import (
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
)

func FuzzRoundTrip(f *testing.F) {
	f.Add("secret")
	f.Fuzz(func(t *testing.T, value string) {
		if !utf8.ValidString(value) {
			t.Skip("JSON replaces invalid UTF-8")
		}

		c := jsonvalue.New[string]()
		encoded, e := c.Encode(value)
		require.NoError(t, e)
		decoded, e := c.Decode(encoded)
		require.NoError(t, e)
		require.Equal(t, value, decoded)
	})
}
