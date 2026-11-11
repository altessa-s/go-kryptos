# go-kryptos

```go
import kryptos "github.com/altessa-s/go-kryptos"
```

Typed secret ownership and a caching manager over memory, Vault KV v2, Google Secret Manager, and Yandex Lockbox.

## Usage

```go
provider, err := memory.New(map[string][]byte{"api-key": []byte("example")})
if err != nil { return err }
store, err := manager.New(provider, manager.WithCacheSize[[]byte](1024))
if err != nil { return err }
defer store.Close(context.WithoutCancel(ctx))
secret, err := store.Get(ctx, "api-key")
if err != nil { return err }
defer secret.Clear()
use(secret.Value)
```

The snippet uses `context`, `providers/memory`, and `manager`. Constructors receive mandatory collaborators positionally and generated options for
tunables.
Consumers schedule `RunUpdateCycle`; the library starts no periodic update loop. `Warm` returns joined failures. `Close` cancels and drains admitted
work.

## Packages and modules

| Package | Responsibility |
|---|---|
| root (`kryptos`) | Secret ownership, redaction, provider/filter/metrics/clock ports, error classes |
| `manager` | Cache, singleflight, retries, writes, update cycles, watches, health, string resolver |
| `codec`, `codec/base64key`, `codec/jsonvalue` | Codec ports and standard adapters |
| `sealbox` | AES-256-GCM sealing of small secrets at rest with a rotating keyring and associated data |
| `providers/memory` | Concurrent in-memory provider |
| `providers/vault` | Separate module; caller-owned Vault client, KV v2 |
| `providers/gcp` | Separate module; credential JSON and client options |
| `providers/lockbox` | Separate module; injected TokenSource and HTTP/gRPC transports |
| `kryptostest` | Contract tests and fault-injecting provider doubles |
| `integration/docker` | Test-only Vault container and go-nomos conformance |

The root requires go-atlas-core v0.3.0 for lifecycle/concurrency and hashicorp/golang-lru/v2 v2.0.7 only inside `internal/lru`.
Cloud SDK dependencies stay in provider modules. Testify v1.12.1 and its indirect dependencies support tests. Go 1.27.4 is pinned.

## Ownership and redaction

`Get`, `Cached`, and provider methods return independent caller-owned secrets. `Put` gives the provider an independent copy.
The manager clears evicted, replaced, purged, and deleted cache entries. Subscribers own delivered events and must call `event.Clear()`.

Clearing overwrites byte slices and invokes `Zeroize()` on cooperating payloads. Strings are dropped, never modified in place.
SDK-owned strings, serialized requests, and runtime copies are outside the clearing guarantee. Prefer `[]byte` for high-value secrets.
Formatting and structured logging redact Secret payloads; JSON excludes both decoded and encoded payloads. Do not log the raw `Value` field.

## Write outcomes

| Outcome | Meaning |
|---|---|
| `nil` | Write committed successfully |
| `ErrConflict` | CAS rejected the write; the caller decides whether to retry |
| `*CommittedError` | Write committed, but cleanup, read-back, or filter maintenance failed; do not retry the write |

The manager refreshes the cache, updates the filter, and emits a change event before returning a committed error. `PutResult` reports whether the
provider found an existing backend key.
Permission and authentication failures return `ErrPermissionDenied`; invalid arguments return `ErrMalformed`; failed preconditions return
`ErrFailedPrecondition`. These and codec errors are terminal.
Vault preserves historical versions by default. GCP and Lockbox resolve concurrent first creation using backend name uniqueness and add versions.

## Verification

```sh
make vet && make lint && make test-all && make test-integration && make generate
make fuzz-seed
make test-integration-docker
```

Unit tests use in-memory HTTP and gRPC transports. Tagged TCP/TLS tests report a skip if listeners are prohibited.
Docker tests require a Docker daemon and use `hashicorp/vault:1.20.0`.
Architecture tests enforce inward dependencies across all modules, generated options, and package documentation.
