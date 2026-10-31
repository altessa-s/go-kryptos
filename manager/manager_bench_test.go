// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"

	kryptos "github.com/altessa-s/go-kryptos"
)

func BenchmarkGet(b *testing.B) {
	p, _ := memory.New(map[string][]byte{"key": make([]byte, 1024)})
	m, _ := manager.New(p)
	defer m.Close(b.Context())
	for b.Loop() {
		s, e := m.Get(b.Context(), "key")
		if e != nil {
			b.Fatal(e)
		}
		s.Clear()
	}
}

func benchReads[T any](b *testing.B, value T) {
	b.Helper()
	p, e := memory.New(map[string]T{"key": value})
	if e != nil {
		b.Fatal(e)
	}
	m, e := manager.New(p)
	if e != nil {
		b.Fatal(e)
	}
	defer m.Close(b.Context())
	for b.Loop() {
		s, e := m.Get(b.Context(), "key")
		if e != nil {
			b.Fatal(e)
		}
		s.Clear()
	}
}

func BenchmarkCacheHits(b *testing.B) {
	b.Run("string", func(b *testing.B) { benchReads(b, "value") })
	b.Run("bytes", func(b *testing.B) { benchReads(b, make([]byte, 1024)) })
	b.Run("struct", func(b *testing.B) { benchReads(b, struct{ Data []byte }{Data: make([]byte, 1024)}) })
}
func BenchmarkCachedMiss(b *testing.B) {
	p, _ := memory.New(map[string]string{})
	m, _ := manager.New(p)
	defer m.Close(b.Context())
	for b.Loop() {
		_, e := m.Cached("missing")
		if !errors.Is(e, kryptos.ErrNotFound) {
			b.Fatal(e)
		}
	}
}
func BenchmarkParallelGet(b *testing.B) {
	for _, count := range []int{1, 1000} {
		for _, shards := range []int{1, 8, 32} {
			b.Run(fmt.Sprintf("keys%d/shards%d", count, shards), func(b *testing.B) {
				values := map[string]string{}
				keys := make([]string, count)
				for i := range keys {
					keys[i] = fmt.Sprintf("key%d", i)
					values[keys[i]] = "value"
				}
				p, _ := memory.New(values)
				m, e := manager.New(p, manager.WithCacheSize[string](4096), manager.WithCacheShards[string](shards))
				if e != nil {
					b.Fatal(e)
				}
				defer m.Close(b.Context())
				if e = m.Warm(b.Context(), keys...); e != nil {
					b.Fatal(e)
				}
				b.ResetTimer()
				b.RunParallel(func(pb *testing.PB) {
					i := 0
					for pb.Next() {
						s, e := m.Get(b.Context(), keys[i%count])
						if e != nil {
							b.Error(e)
							return
						}
						s.Clear()
						i++
					}
				})
			})
		}
	}
}
func BenchmarkStampede(b *testing.B) {
	for b.Loop() {
		p, _ := memory.New(map[string]string{"key": "value"})
		var calls atomic.Int32
		decorated := &kryptostest.Provider[string]{Backend: p, GetFunc: func(ctx context.Context, key string) (*kryptos.Secret[string], error) {
			calls.Add(1)
			return p.Get(ctx, key)
		}}
		m, _ := manager.New(decorated)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range 1000 {
			wg.Go(func() {
				<-start
				s, e := m.Get(b.Context(), "key")
				if e != nil {
					b.Error(e)
					return
				}
				s.Clear()
			})
		}
		close(start)
		wg.Wait()
		if calls.Load() != 1 {
			b.Fatalf("provider calls: %d", calls.Load())
		}
		_ = m.Close(b.Context())
	}
}
func BenchmarkReconcile(b *testing.B) {
	for _, count := range []int{1000, 10000} {
		for _, watchers := range []int{0, 100} {
			b.Run(fmt.Sprintf("keys%d/watchers%d", count, watchers), func(b *testing.B) {
				values := map[string]string{}
				for i := range count {
					values[fmt.Sprintf("key%d", i)] = "value"
				}
				p, _ := memory.New(values)
				m, e := manager.New(p, manager.WithCacheSize[string](count))
				if e != nil {
					b.Fatal(e)
				}
				defer m.Close(b.Context())
				if e = m.RunUpdateCycle(b.Context()); e != nil {
					b.Fatal(e)
				}
				for range watchers {
					w, e := m.Watch(b.Context(), manager.WithSkipInitial())
					if e != nil {
						b.Fatal(e)
					}
					defer w.Close()
				}
				for b.Loop() {
					b.StopTimer()
					for i := range count / 100 {
						if e = p.Put(b.Context(), fmt.Sprintf("key%d", i), "changed"); e != nil {
							b.Fatal(e)
						}
					}
					b.StartTimer()
					if e = m.RunUpdateCycle(b.Context()); e != nil {
						b.Fatal(e)
					}
				}
			})
		}
	}
}
