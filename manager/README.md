# manager

```go
import "github.com/altessa-s/go-kryptos/manager"
```

`New[T](provider, options...)` validates configuration and returns a lifecycle-owned manager.

| Method | Contract |
|---|---|
| `Get`, `Cached` | Caller-owned copies; Cached never fetches |
| `GetShared` | Borrowed reference; invalidated by cache mutation |
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
