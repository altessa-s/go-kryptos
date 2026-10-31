# lockbox

The provider accepts injected credentials and transport settings. Results are caller-owned and must be cleared. SDK-owned copies cannot be zeroed.

Concurrent first writes use backend name uniqueness. Unconditional writes add versions; concurrent deletion may return ErrNotFound.

## Creation concurrency

Two independent providers may both observe an absent key. The backend enforces unique names. The losing creator resolves AlreadyExists,
then appends a version to the existing secret. Each successful Put creates one version. Concurrent Put/Delete may return ErrNotFound;
the manager does not introduce distributed locks. Version history is retained rather than destroyed automatically.

## Configuration and ownership

Key and value codecs default to RawURL base64 and JSON. Labels filter listings, and WithIgnoreInvalidKeys can skip foreign names.
Payload copies returned by Get/List belong to the caller. Raw SDK errors and codec messages are sanitized at the adapter boundary.

WithEndpoints and WithDialOptions configure the secret, payload, and operation services. TokenSource ownership remains with the consumer.
NewServiceAccountTokens copies the RSA key, signs PS256 JWTs, and exchanges them through an injected httpx.Doer. Responses are status-checked
and bounded to 64 KiB. Refresh is shared and lifecycle-bound; callers wait for refresh once the early-refresh window begins. Close cancels
refresh and clears owned key bytes. Strings containing IAM tokens are dropped, not overwritten.

`PutResult` returns `WriteResult.Created` from backend existence before the write. `Put` discards that result. Close releases owned resources even with
a canceled context.

Accepted asynchronous mutations are polled using the same operation ID. Three consecutive transient polling failures, cancellation,
or an unreadable operation result return `kryptos.ErrIndeterminate`. Commitment is unknown: callers must reconcile backend state before
another write and must not automatically retry the mutation. `CommittedError` is reserved for confirmed writes with failed cleanup.

Encoded payloads are always written as `BinaryValue` in the `Data` entry. Reads accept binary values
and fall back to `TextValue` for externally created secrets; an absent value variant is malformed.
Metadata names are revalidated after listing. A concurrent rename returns `ErrConflict` and invalidates
the lookup mapping, so the operation cannot read or mutate the renamed secret through the stale key.

Ambiguous `Create/AddVersion` RPC failures return `kryptos.ErrIndeterminate`, including lost responses, cancellation, and deadlines after dispatch.
Definitive backend rejections keep their error classes. Reconcile backend state before issuing another write; do not automatically replay it.
