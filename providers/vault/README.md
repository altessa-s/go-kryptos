# vault

```go
import "github.com/altessa-s/go-kryptos/providers/vault"
```

`New[T](client, options...)` accepts a caller-owned `*api.Client`. Build that client with your application's transport and credentials.
The provider never reads files or environment. Closing a provider stops its admission without closing the shared client.

## Options

| Option | Default |
|---|---|
| `WithMountPath` | `kv` |
| `WithSecretPath` | `blitz` |
| `WithCheckAndSet` | Disabled |
| `WithDestroyPreviousVersions` | Disabled |
| `WithKeyCodec`, `WithValueCodec` | RawURL base64, JSON |

## Writes and permissions

Without CAS, writes are last-writer-wins version additions. With CAS, the provider reads current metadata and sends that version to KV v2;
conflicts return `kryptos.ErrConflict` without retry. Two independent first writers may conflict; the caller retries the loser explicitly.
A concurrent deletion can also invalidate a CAS version. Cleanup failure after a committed write returns `CommittedError` wrapping `ErrCleanupFailed`.

`Ping` calls token lookup-self, not sys/mounts. Applications need their KV data/metadata permissions and token lookup-self.
History destruction requires additional destroy permissions only when explicitly enabled. Delete permanently removes metadata and versions.
SDK-owned JSON strings cannot be cleared; caller-owned results and temporary byte buffers are cleared separately.

`PutResult` returns `WriteResult.Created` from backend existence before the write. `Put` discards that result. Close releases owned resources even with
a canceled context.

Encoded payload bytes are always stored as standard base64 in the KV data map's `data` string.
Reads require that format; legacy plaintext data is not supported. A successful HTTP write with
unreadable version metadata returns `CommittedError` wrapping `ErrMalformed`; the manager refreshes
its cache without retrying. Ambiguous transport/server failures return `ErrIndeterminate`, requiring
backend reconciliation before another mutation. The write uses a client clone with SDK retries disabled.
