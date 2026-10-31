// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package base64key

import "encoding/base64"

// Codec implements unpadded URL-safe base64 keys.
type Codec struct{}

// New returns a key codec.
func New() *Codec { return &Codec{} }
func (Codec) Encode(v string) (string, error) {
	return base64.RawURLEncoding.EncodeToString([]byte(v)), nil
}
func (Codec) Decode(v string) (string, error) {
	b, e := base64.RawURLEncoding.DecodeString(v)
	return string(b), e
}
