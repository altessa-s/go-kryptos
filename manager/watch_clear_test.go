// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

type trackedCopies struct {
	mu      sync.Mutex
	buffers [][]byte
}
type clearingPayload struct {
	Data   []byte
	Copies *trackedCopies
}

func (p clearingPayload) Clone() clearingPayload {
	data := append([]byte(nil), p.Data...)
	p.Copies.mu.Lock()
	p.Copies.buffers = append(p.Copies.buffers, data)
	p.Copies.mu.Unlock()
	return clearingPayload{Data: data, Copies: p.Copies}
}
func (p clearingPayload) Zeroize() { clear(p.Data) }
func TestEveryDroppedWatchCopyIsCleared(t *testing.T) {
	t.Parallel()
	for _, policy := range []manager.OverflowPolicy{manager.DropNewest, manager.DropOldest} {
		t.Run(map[manager.OverflowPolicy]string{manager.DropNewest: "newest", manager.DropOldest: "oldest"}[policy], func(t *testing.T) {
			t.Parallel()
			copies := &trackedCopies{}
			p, e := memory.New[clearingPayload](nil)
			require.NoError(t, e)
			m, e := manager.New(p)
			require.NoError(t, e)
			s, e := m.Watch(t.Context(), manager.WithBuffer(1), manager.WithOverflow(policy))
			require.NoError(t, e)
			for range 4 {
				require.NoError(t, m.Put(t.Context(), "key", clearingPayload{Data: []byte("sensitive"), Copies: copies}))
			}
			s.Close()
			require.NoError(t, m.Close(t.Context()))
			copies.mu.Lock()
			defer copies.mu.Unlock()
			require.NotEmpty(t, copies.buffers)
			for _, b := range copies.buffers {
				require.Equal(t, make([]byte, len(b)), b)
			}
		})
	}
}

type watchPanicPayload struct {
	clearingPayload
	remaining *atomic.Int32
}

func (p watchPanicPayload) Clone() watchPanicPayload {
	if p.remaining.Add(-1) == 0 {
		panic("initial clone failed")
	}
	p.clearingPayload = p.clearingPayload.Clone()
	return p
}
func TestWatchInitialPanicRollsBack(t *testing.T) {
	t.Parallel()
	copies := &trackedCopies{}
	count := &atomic.Int32{}
	p, err := memory.New[watchPanicPayload](nil)
	require.NoError(t, err)
	m, err := manager.New(p)
	require.NoError(t, err)
	defer m.Close(t.Context())
	value := watchPanicPayload{Data: []byte("secret"), Copies: copies, remaining: count}
	require.NoError(t, m.Put(t.Context(), "first", value))
	require.NoError(t, m.Put(t.Context(), "second", value))
	copies.mu.Lock()
	copies.buffers = nil
	copies.mu.Unlock()
	count.Store(2)
	sub, err := m.Watch(t.Context())
	require.Nil(t, sub)
	require.ErrorIs(t, err, kryptos.ErrPanicked)
	copies.mu.Lock()
	require.Len(t, copies.buffers, 1)
	require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, copies.buffers[0])
	copies.mu.Unlock()
	// A failed subscription must not receive later writes.
	count.Store(100)
	require.NoError(t, m.Put(t.Context(), "third", value))
	require.Equal(t, int32(95), count.Load())
}
