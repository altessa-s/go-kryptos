// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package keystate_test

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/internal/keystate"
)

func TestSerializesSameKey(t *testing.T) {
	t.Parallel()

	var (
		locks keystate.Locks
		wg    sync.WaitGroup
	)

	value := 0

	for range 100 {
		wg.Go(func() { release := locks.Lock("same"); value++; release() })
	}

	wg.Wait()
	require.Equal(t, 100, value)

	a := locks.Lock("a")
	b := locks.Lock("b")
	b()
	a()
}
