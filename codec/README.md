# codec

```go
import "github.com/altessa-s/go-kryptos/codec"
```

Secret payloads must never be logged. Callers clear owned results after use. Strings cannot be reliably zeroed; prefer byte slices.

`ValueCodec` supports arbitrary bytes, including invalid UTF-8. Both encoded buffers and decoded
results transfer ownership to the provider even when returned with an error. A decoded result may
alias the input; it must not share mutable state retained by the codec. Providers clear rejected
results and clear successful results after making a caller-owned copy, including if cloning panics.
