# sealbox

```go
import "github.com/altessa-s/go-kryptos/sealbox"
```

Seals and opens small secrets at rest (API keys, webhook secrets, client credentials) with AES-256-GCM and a keyring of base64-encoded
32-byte master keys. The current key seals, previous keys still open values sealed before a rotation, and every value is bound to
caller-chosen associated data.

## Key types

| Symbol                  | Description                                                                                                  |
|-------------------------|--------------------------------------------------------------------------------------------------------------|
| `New(current, prev...)` | Builds a `*Box`; an empty current key returns a nil box (no error): `Seal`/`Open` fail with `ErrNoKey`, `Current`/`Opens` report false |
| `Box.Seal(p, aad)`      | Encrypts with the current key and a fresh random nonce                                                       |
| `Box.Open(s, aad)`      | Decrypts with the key named by `s.KeyID`; `ErrUnknownKey` for a key not configured, `ErrOpen` otherwise      |
| `Box.Current(s)`        | Whether `s` was sealed with the current key (re-seal values that are not)                                   |
| `Box.Opens(s)`          | Whether the box holds the key `s` was sealed with (no authentication)                                       |
| `Sealed`                | `KeyID`, `Nonce`, `Ciphertext` — store all three; `Set()` reports a non-empty value                          |
| `KeyID(key)`            | Key identifier: first 16 hex digits of `sha256(key)`                                                         |

## Format

| Field        | Content                                                     |
|--------------|-------------------------------------------------------------|
| `KeyID`      | `hex(sha256(rawKey))[:16]`                                  |
| `Nonce`      | 12 random bytes per seal                                    |
| `Ciphertext` | GCM output: encrypted plaintext followed by the 16-byte tag |

The format is stable; a golden-vector test pins it so values stored by earlier versions keep opening.

## Rotation

1. Generate a new key (`openssl rand -base64 32`), configure it as current and move the old key to the previous keys.
2. Re-seal values for which `Box.Current` is false when they are next saved (or in a background pass).
3. Drop the old key once no stored value uses it.

**Nonce budget.** Nonces are 96 random bits, so a key is good for at most 2^32 seals (NIST SP 800-38D); past that a nonce collision, which breaks
GCM for that key, becomes likelier than 2^-32. The box cannot count seals across processes and restarts, so rotate the current key well before 2^32
seals with it in total. Sealing secrets on save stays far below the limit; per-request or per-message sealing needs a rotation schedule derived from
the seal rate.

## Associated data

Bind every value to the record identity **and** the field, for example `[]byte("provider:" + id + ":apiKey")`, so a ciphertext cannot be copied
onto another record or swapped between two sealed fields of the same record.

## Errors

| Error           | Meaning                                                           |
|-----------------|-------------------------------------------------------------------|
| `ErrNoKey`      | No key configured (nil box, or a zero `Box` not built by `New`)   |
| `ErrUnknownKey` | The value was sealed with a key that is not configured            |
| `ErrOpen`       | The value does not open: nonce of the wrong length, or it does not authenticate (tampered, wrong associated data) |

Key-loading errors name the key's position (0 = current), never its value.
