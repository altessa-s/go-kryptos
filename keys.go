// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos

// MinKeyLength and MaxKeyLength bound ASCII secret identifiers.
const (
	MinKeyLength = 2
	MaxKeyLength = 255
)

// ValidateKey accepts letters, digits, underscores, periods, colons, and hyphens, excluding traversal names.
func ValidateKey(key string) error {
	if len(key) < MinKeyLength || len(key) > MaxKeyLength || key == ".." {
		return ErrInvalidKey
	}
	for _, c := range []byte(key) {
		allowed := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '.' || c == ':' || c == '-'
		if !allowed {
			return ErrInvalidKey
		}
	}
	return nil
}
