// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos_test

import (
	"testing"

	kryptos "github.com/altessa-s/go-kryptos"
)

func BenchmarkCloneAndClear(b *testing.B) {
	s := kryptos.NewSecret("key", make([]byte, 1024), nil, "1")
	defer s.Clear()
	for b.Loop() {
		v := s.Clone()
		v.Clear()
	}
}
func BenchmarkValidateKey(b *testing.B) {
	for b.Loop() {
		if e := kryptos.ValidateKey("namespace:secret-key"); e != nil {
			b.Fatal(e)
		}
	}
}
