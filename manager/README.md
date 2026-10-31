# manager

```go
import "github.com/altessa-s/go-kryptos/manager"
```

`New[T](provider, options...)` validates configuration and returns a lifecycle-owned manager.

| Method | Contract |
|---|---|
| `Get`, `Cached` | Caller-owned copies; Cached never fetches |
| `Put`, `Delete` | Backend mutation, cache invalidation/refresh, filter update, watch event |
| `Warm` | Bounded concurrent fetches, joined per-key errors |
| `RunUpdateCycle` | Caller-driven reconciliation; overlaps collapse |
| `Watch` | Buffered Created/Updated/Deleted events; no source filter or error events |
| `Purge`, `Len`, `LastSync`, `Health` | Cache management and observation |
| `Close` | Stop admission, cancel shared work, drain operations, close provider once |

## Options

Generated options configure cache size/shards, retry count/backoff, clock, logger, metrics, filter ports, empty-list threshold,
cycle timeout, stale-health threshold, and explicit shallow-clone opt-in. Invalid numeric settings fail construction.
Watch options select keys, types, buffer size, drop-newest/drop-oldest overflow, and whether cached initial values are emitted.

## Concurrency and ownership

Singleflight uses the core caller-independent lifecycle; the last canceled waiter cancels shared work. A sanitized recover boundary prevents
provider/decoder panic values reaching errors, logs, metrics, or waiters. Per-key mutation locks serialize reads with writes. Reconciliation
excludes mutations while applying a consistent listing; version metadata tracks changes independently of LRU eviction.

The manager owns cached values. Consumers clear Get results and delivered events. Closing subscriptions clears queued events.
StringResolver implements the go-nomos resolver contract structurally; neither production library imports the other.

Get and Cached return independent copies that remain valid across eviction, replacement, and Close. Callers own and clear them. Reads retain no encoded
payload. Put returns CommittedError for any failure after commitment; do not retry that write.

An indeterminate Put or Delete is never retried. Its cached value is invalidated, and the next Get bypasses a potentially stale negative filter to read backend
state. No success event is emitted for an unknown outcome. Cache-hit cloning panics return ErrPanicked; cloning failures while installing a committed
write, reading it back, cloning its fallback, or notifying watchers return CommittedError wrapping ErrPanicked. These errors omit the panic value and release the cache lock.

An unknown deletion marks the key dirty so an overlapping listing cannot restore stale data. It retains backend lookup eligibility without emitting
a confirmed deletion event. A read-back panic after Put still reconciles the filter, cache, and watch from the written value when cloning succeeds.
