// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package codec

// KeyCodec transforms storage identifiers in both directions.
type KeyCodec interface {
	Encode(string) (string, error)
	Decode(string) (string, error)
}

// ValueCodec transforms payloads in both directions.
// Encode receives a provider-owned payload copy. Returned bytes transfer ownership
// to the provider, even on error, and may alias that copy. The provider clears both
// after the write; codecs must not return shared or retained buffers.
type ValueCodec[T any] interface {
	Encode(T) ([]byte, error)
	Decode([]byte) (T, error)
}
