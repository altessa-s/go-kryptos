// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package zero

import "reflect"

// Clear overwrites byte payloads or delegates to the payload's Zeroize method.
// Strings and other values are dropped, never modified through unsafe aliases.
func Clear[T any](v *T) {
	if z, ok := any(*v).(interface{ Zeroize() }); ok {
		r := reflect.ValueOf(z)
		if r.Kind() != reflect.Pointer || !r.IsNil() {
			z.Zeroize()
		}
	} else if b, ok := any(*v).([]byte); ok {
		clear(b)
	}
	var empty T
	*v = empty
}
