// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package memory_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/memory"
)

type copyTracker struct {
	calls  int
	failAt int
	copies [][]byte
}
type trackedPayload struct {
	data    []byte
	tracker *copyTracker
}

func (p trackedPayload) Clone() trackedPayload {
	p.tracker.calls++
	if p.tracker.calls == p.tracker.failAt {
		panic("clone failed")
	}
	p.data = slices.Clone(p.data)
	p.tracker.copies = append(p.tracker.copies, p.data)
	return p
}
func (p *trackedPayload) Zeroize() { clear(p.data) }
func TestListClearsCopiesOnPanic(t *testing.T) {
	t.Parallel()
	tracker := &copyTracker{}
	p, err := memory.New(map[string]trackedPayload{
		"first":  {data: []byte("secret"), tracker: tracker},
		"second": {data: []byte("secret"), tracker: tracker},
	})
	require.NoError(t, err)
	defer p.Close(t.Context())
	tracker.failAt = tracker.calls + 2
	tracker.copies = nil
	require.Panics(t, func() { _, _ = p.List(t.Context()) })
	require.Len(t, tracker.copies, 1)
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, tracker.copies[0])
	got, err := p.Get(t.Context(), "first")
	require.NoError(t, err)
	require.Equal(t, []byte("secret"), got.Value.data)
	got.Clear()
}

func TestConstructorClearsCopiesOnPanic(t *testing.T) {
	t.Parallel()
	tracker := &copyTracker{failAt: 2}
	require.Panics(t, func() {
		_, _ = memory.New(map[string]trackedPayload{
			"first":  {data: []byte("secret"), tracker: tracker},
			"second": {data: []byte("secret"), tracker: tracker},
		})
	})
	require.Len(t, tracker.copies, 1)
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, tracker.copies[0])
}
