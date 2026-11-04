// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

//go:generate optgen generate
import (
	"log/slog"
	"time"

	"github.com/altessa-s/go-atlas-core/retry"

	kryptos "github.com/altessa-s/go-kryptos"
)

const (
	// DefaultBackoff is the initial retry delay when no backoff is set.
	DefaultBackoff = 10 * time.Millisecond
	// DefaultJitter is the jitter fraction of the default backoff.
	DefaultJitter = 0.2
	// DefaultWarmConcurrency bounds the parallel loads of Warm.
	DefaultWarmConcurrency = 8
	// DefaultCacheSize is the default cache capacity.
	DefaultCacheSize = 1024
	// DefaultCacheShards is the default number of cache shards.
	DefaultCacheShards = 1
	// DefaultMaxRetries is the default retry limit.
	DefaultMaxRetries = 3
	// DefaultEmptyListingThreshold is the default empty-listing threshold.
	DefaultEmptyListingThreshold = 3
	// DefaultCycleTimeout is the default timeout of one cycle.
	DefaultCycleTimeout = 30 * time.Second
	// DefaultStaleAfter is the default staleness age.
	DefaultStaleAfter = 10 * time.Minute
)

type options[T any] struct {
	logger                *slog.Logger
	maxRetries            int `optgen:"default=DefaultMaxRetries"`
	backoff               retry.NextDelayFunc
	cacheSize             int `optgen:"default=DefaultCacheSize"`
	cacheShards           int `optgen:"default=DefaultCacheShards"`
	keyFilter             kryptos.KeyFilter
	filterRebuilder       kryptos.FilterRebuilder
	filterDeleter         kryptos.FilterDeleter
	metrics               kryptos.Metrics
	clock                 kryptos.Clock
	emptyListingThreshold int           `optgen:"default=DefaultEmptyListingThreshold"`
	cycleTimeout          time.Duration `optgen:"default=DefaultCycleTimeout" optval:"nonpositive"`
	staleAfter            time.Duration `optgen:"default=DefaultStaleAfter" optval:"nonpositive"`
	allowShallowClone     bool
}
