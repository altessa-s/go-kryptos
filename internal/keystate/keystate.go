// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package keystate

import "sync"

// Locks serializes mutations and fetches of the same key without retaining idle keys.
type Locks struct {
	mu   sync.Mutex
	keys map[string]*entry
}
type entry struct {
	mu   sync.Mutex
	refs int
}

// Lock returns a release function that must be called exactly once.
func (l *Locks) Lock(key string) func() {
	l.mu.Lock()
	if l.keys == nil {
		l.keys = make(map[string]*entry)
	}

	e := l.keys[key]
	if e == nil {
		e = &entry{}
		l.keys[key] = e
	}

	e.refs++
	l.mu.Unlock()
	e.mu.Lock()

	return func() {
		e.mu.Unlock()
		l.mu.Lock()

		e.refs--
		if e.refs == 0 {
			delete(l.keys, key)
		}
		l.mu.Unlock()
	}
}
