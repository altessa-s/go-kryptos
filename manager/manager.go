// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"
	"sync"
	"time"

	"github.com/altessa-s/go-atlas-core/retry"
	"github.com/altessa-s/go-atlas-core/runtime/cycle"
	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	"github.com/altessa-s/go-kryptos"
	"github.com/altessa-s/go-kryptos/internal/clone"
	"github.com/altessa-s/go-kryptos/internal/keystate"
	"github.com/altessa-s/go-kryptos/internal/lru"
)

// Manager coordinates ownership, caching, writes, reconciliation, and subscriptions.
type Manager[T any] struct {
	p         kryptos.Provider[T]
	o         *options[T]
	mu        sync.Mutex
	cache     *lru.Cache[*kryptos.Secret[T]]
	keys      keystate.Locks
	reconcile sync.RWMutex
	group     *singleflight.Group[string, struct{}]
	guard     *cycle.Guard
	policy    *retry.Policy
	life      context.Context
	cancel    context.CancelFunc
	closed    bool
	active    sync.WaitGroup
	closeOnce sync.Once
	done      chan struct{}
	closeErr  error
	lastSync  time.Time
	empty     int
	watchers  map[*Subscription[T]]struct{}
	versions  map[string]string
	dirty     map[string]bool
}

// New constructs a manager without starting a background update loop.
func New[T any](p kryptos.Provider[T], opts ...Option[T]) (*Manager[T], error) {
	if p == nil || reflect.ValueOf(p).Kind() == reflect.Pointer && reflect.ValueOf(p).IsNil() {
		return nil, kryptos.ErrInvalidOptions
	}
	o := newOptions(opts...)
	if !o.allowShallowClone && clone.CheckType(reflect.TypeFor[T]()) != nil {
		return nil, kryptos.ErrUncloneablePayload
	}
	if o.cacheSize < 1 || o.cacheShards < 1 || o.cacheShards > o.cacheSize || o.maxRetries < 0 ||
		o.cycleTimeout <= 0 || o.staleAfter <= 0 || o.emptyListingThreshold < 1 {
		return nil, kryptos.ErrInvalidOptions
	}
	if o.clock == nil {
		o.clock = kryptos.RealClock{}
	}
	if o.logger == nil {
		o.logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.metrics == nil {
		o.metrics = noMetrics{}
	}
	if o.backoff == nil {
		o.backoff = retry.Exponential(DefaultBackoff, retry.WithExponentialMaxDelay(time.Minute), retry.WithExponentialJitter(DefaultJitter))
	}
	life, cancel := context.WithCancel(context.Background())
	c, e := lru.New(o.cacheSize, o.cacheShards, func(_ string, s *kryptos.Secret[T]) { s.Clear() })
	if e != nil {
		cancel()
		return nil, e
	}
	m := &Manager[T]{p: p, o: o, cache: c, life: life, cancel: cancel, done: make(chan struct{}), watchers: make(map[*Subscription[T]]struct{}),
		versions: make(map[string]string)}
	m.group = singleflight.New[string, struct{}](singleflight.WithLifecycle(life), singleflight.WithTimeout(o.cycleTimeout))
	m.guard = cycle.New(m.update)
	m.policy = retry.NewPolicy(retry.WithMaxAttempts(o.maxRetries), retry.WithNextDelay(o.backoff), retry.WithShouldRetry(retryable))
	return m, nil
}
func retryable(e error) bool {
	if _, ok := errors.AsType[*kryptos.CommittedError](e); ok {
		return false
	}
	for _, terminal := range []error{kryptos.ErrConflict, kryptos.ErrNotFound, kryptos.ErrInvalidKey, kryptos.ErrClosed,
		kryptos.ErrIndeterminate, kryptos.ErrPanicked, kryptos.ErrReadOnly, kryptos.ErrUncloneablePayload, kryptos.ErrMalformed, kryptos.ErrInvalidOptions,
		kryptos.ErrPermissionDenied, kryptos.ErrFailedPrecondition, kryptos.ErrKeyEncoding, kryptos.ErrKeyDecoding,
		kryptos.ErrValueEncoding, kryptos.ErrValueDecoding, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(e, terminal) {
			return false
		}
	}
	if errors.Is(e, kryptos.ErrUnavailable) {
		return true
	}
	if transient, ok := errors.AsType[interface {
		error
		Temporary() bool
	}](e); ok {
		return transient.Temporary()
	}
	return false
}

type noMetrics struct{}

func (noMetrics) Skipped()                     {}
func (noMetrics) FilterFailed()                {}
func (noMetrics) CacheHit()                    {}
func (noMetrics) CacheMiss()                   {}
func (noMetrics) Fetched(time.Duration, error) {}
func (noMetrics) Cycled(time.Duration, error)  {}
func (noMetrics) CacheSize(int)                {}
