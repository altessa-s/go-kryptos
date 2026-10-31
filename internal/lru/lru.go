// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lru

import (
	"hash/maphash"

	lru "github.com/hashicorp/golang-lru/v2"
)

// Cache divides capacity across shards. Callers synchronize access to payloads.
type Cache[V any] struct {
	seed   maphash.Seed
	shards []*lru.Cache[string, V]
}

func New[V any](size, shards int, evict func(string, V)) (*Cache[V], error) {
	c := &Cache[V]{seed: maphash.MakeSeed()}
	for i := range shards {
		n := size / shards
		if i < size%shards {
			n++
		}
		s, e := lru.NewWithEvict[string, V](n, evict)
		if e != nil {
			return nil, e
		}
		c.shards = append(c.shards, s)
	}
	return c, nil
}
func (c *Cache[V]) shard(k string) *lru.Cache[string, V] {
	return c.shards[maphash.String(c.seed, k)%uint64(len(c.shards))]
}
func (c *Cache[V]) Get(k string) (V, bool) { return c.shard(k).Get(k) }
func (c *Cache[V]) Put(k string, v V)      { s := c.shard(k); s.Remove(k); s.Add(k, v) }
func (c *Cache[V]) Remove(k string)        { c.shard(k).Remove(k) }
func (c *Cache[V]) Purge() {
	for _, s := range c.shards {
		s.Purge()
	}
}
func (c *Cache[V]) Len() int {
	n := 0
	for _, s := range c.shards {
		n += s.Len()
	}
	return n
}
func (c *Cache[V]) Keys() []string {
	var keys []string
	for _, s := range c.shards {
		keys = append(keys, s.Keys()...)
	}
	return keys
}
