// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package base64key_test

import (
	"testing"

	"github.com/altessa-s/go-kryptos/codec/base64key"
)

func BenchmarkRoundTrip(b *testing.B) {
	c := base64key.New()
	for b.Loop() {
		v, e := c.Encode("secret")
		if e != nil {
			b.Fatal(e)
		}

		if _, e = c.Decode(v); e != nil {
			b.Fatal(e)
		}
	}
}
