# memory

```go
import "github.com/altessa-s/go-kryptos/providers/memory"
```

Secret payloads must never be logged. Callers clear owned results after use. Strings cannot be reliably zeroed; prefer byte slices.

The memory backend always rejects uncloneable payloads. Manager AllowShallowClone applies only to manager validation; it does not weaken the
independently constructed backend’s ownership guarantee. Supply a Clone method for mutable custom payloads.
