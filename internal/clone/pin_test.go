// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package clone_test

import (
	"maps"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/clone"
)

// clonedMap is a reference type with its own Clone method.
type clonedMap map[string]string

func (m clonedMap) Clone() clonedMap {
	out := make(clonedMap, len(m))
	maps.Copy(out, m)

	return out
}

func TestCloneSecret_Pin(t *testing.T) {
	t.Parallel()

	t.Run("arrays are copied element by element", func(t *testing.T) {
		t.Parallel()

		orig := [2][]byte{[]byte("a"), []byte("b")}
		c := clone.Copy(orig)
		require.Equal(t, orig, c)
		c[0][0] = 'X'

		require.Equal(t, "a", string(orig[0]))
	})

	t.Run("a cloner reference reached twice is cloned once", func(t *testing.T) {
		t.Parallel()

		shared := clonedMap{"k": "v"}
		orig := struct{ A, B clonedMap }{shared, shared}
		c := clone.Copy(orig)
		require.Equal(t, reflect.ValueOf(c.A).Pointer(), reflect.ValueOf(c.B).Pointer())
		require.NotEqual(t, reflect.ValueOf(shared).Pointer(), reflect.ValueOf(c.A).Pointer())
		c.A["k"] = "changed"

		require.Equal(t, "v", shared["k"])
	})
}

func TestShallowValuePath_Pin(t *testing.T) {
	t.Parallel()

	fn := func() {}
	for _, tc := range []struct {
		name    string
		payload any
		bad     string // full reported path; empty when cloneable
	}{
		{"nil interface field", struct{ I any }{}, ""},
		{"nil chan", struct{ C chan int }{}, ""},
		{"nil references", struct {
			P *int
			M map[string]int
			S []int
		}{}, ""},
		{"byte slice", []byte("raw"), ""},
		{"map value func", map[string]any{"k": fn}, "map[string]interface {}[value](func()) (func)"},
		{"map key chan", map[any]int{make(chan int): 1}, "map[interface {}]int[key](chan int) (chan)"},
		{"array element chan", [2]any{1, make(chan int)}, "[2]interface {}[elem](chan int) (chan)"},
		{"unexported reference", struct{ s []int }{}, "struct { s []int }.s (unexported []int)"},
		{"pointer to cloner", &withClone{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := clone.Check(tc.payload)
			if tc.bad == "" {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, clone.ErrUncloneable)
			require.Error(t, err)
		})
	}
}
