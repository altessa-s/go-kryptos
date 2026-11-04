// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package jsonvalue

import "encoding/json"

// Codec implements JSON payload encoding.
type Codec[T any] struct{}

// New returns a JSON codec.
func New[T any]() *Codec[T] { return &Codec[T]{} }

// Encode marshals v to JSON.
func (Codec[T]) Encode(v T) ([]byte, error) { return json.Marshal(v) }

// Decode unmarshals JSON into a new T.
func (Codec[T]) Decode(b []byte) (T, error) { var v T; e := json.Unmarshal(b, &v); return v, e }
