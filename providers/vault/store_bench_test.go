// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import "testing"

func BenchmarkDecode(b *testing.B) {
	p := typedProvider[[]byte](b)
	defer p.Close(b.Context())
	if e := p.Put(b.Context(), "key", make([]byte, 1024)); e != nil {
		b.Fatal(e)
	}
	for b.Loop() {
		s, e := p.Get(b.Context(), "key")
		if e != nil {
			b.Fatal(e)
		}
		s.Clear()
	}
}
