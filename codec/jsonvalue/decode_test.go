// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package jsonvalue_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
)

func roundTripDecoded[T any](t *testing.T, b []byte) {
	t.Helper()
	c := jsonvalue.New[T]()
	v, e := c.Decode(b)
	if e != nil {
		return
	}
	out, e := c.Encode(v)
	require.NoError(t, e)
	again, e := c.Decode(out)
	require.NoError(t, e)
	require.Equal(t, v, again)
}
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"data":"value"}`))
	f.Add([]byte(`"value"`))
	f.Add([]byte(`null`))
	f.Fuzz(func(t *testing.T, b []byte) {
		roundTripDecoded[string](t, b)
		roundTripDecoded[map[string]string](t, b)
		roundTripDecoded[struct{ Data string }](t, b)
	})
}
