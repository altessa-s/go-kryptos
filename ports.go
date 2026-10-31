// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos

import (
	"context"
	"iter"
	"time"
)

// Getter retrieves caller-owned secret copies.
type Getter[T any] interface {
	Get(context.Context, string) (*Secret[T], error)
}

// Lister enumerates caller-owned secret copies.
type Lister[T any] interface {
	List(context.Context) ([]*Secret[T], error)
}

// Writer changes a backend. ErrConflict must remain observable.
type Writer[T any] interface {
	Put(context.Context, string, T) error
	Delete(context.Context, string) error
}

// Provider is a complete secret backend; Close must be idempotent.
type Provider[T any] interface {
	Getter[T]
	Lister[T]
	Writer[T]
	Name() string
	Ping(context.Context) error
	Close(context.Context) error
}

// KeyFilter is an advisory existence index.
type KeyFilter interface {
	MightExist(context.Context, string) (bool, error)
	Add(context.Context, string) error
}

// FilterRebuilder atomically rebuilds while journaling concurrent additions.
type FilterRebuilder interface {
	Rebuild(context.Context, func(context.Context) iter.Seq2[string, error]) error
}

// FilterDeleter removes an index entry when rebuilding is unavailable.
type FilterDeleter interface {
	Delete(context.Context, string) error
}

// Metrics receives operation outcomes without secret values.
type Metrics interface {
	CacheHit()
	CacheMiss()
	Fetched(time.Duration, error)
	Cycled(time.Duration, error)
	CacheSize(int)
}

// Clock supplies observation time.
type Clock interface{ Now() time.Time }

// RealClock uses the system clock.
type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now() }

// HealthStatus describes manager availability.
type HealthStatus int

const (
	Serving HealthStatus = iota
	Degraded
	NotServing
)

// EventType identifies a change.
type EventType int

const (
	Created EventType = iota
	Updated
	Deleted
)

// Event transfers ownership of Secret to a subscriber.
type Event[T any] struct {
	Type   EventType
	Key    string
	Secret *Secret[T] `json:"-"`
	Time   time.Time
}

// Clear releases an event's payload.
func (e *Event[T]) Clear() {
	if e.Secret != nil {
		e.Secret.Clear()
		e.Secret = nil
	}
}
