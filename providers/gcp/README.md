# gcp

The provider accepts injected credentials and transport settings. Results are caller-owned and must be cleared. SDK-owned copies cannot be zeroed.

Concurrent first writes use backend name uniqueness. Unconditional writes add versions; concurrent deletion may return ErrNotFound.

## Creation concurrency

Two independent providers may both observe an absent key. The backend enforces unique names. The losing creator resolves AlreadyExists,
then appends a version to the existing secret. Each successful Put creates one version. Concurrent Put/Delete may return ErrNotFound;
the manager does not introduce distributed locks. Version history is retained rather than destroyed automatically.

## Configuration and ownership

Key and value codecs default to RawURL base64 and JSON. Labels filter listings, and WithIgnoreInvalidKeys can skip foreign names.
Payload copies returned by Get/List belong to the caller. Raw SDK errors and codec messages are sanitized at the adapter boundary.

Credentials must be service-account JSON. WithClientOptions supports endpoint, connection, and authentication overrides for tests.
WithClearCredentials explicitly clears the supplied credential slice after construction; otherwise it remains caller-owned.

`PutResult` returns `WriteResult.Created` from backend existence before the write. `Put` discards that result. Close releases owned resources even with
a canceled context.
