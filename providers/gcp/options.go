// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp

//go:generate optgen generate
import (
	"github.com/altessa-s/go-kryptos/codec"

	"google.golang.org/api/option"

	kryptos "github.com/altessa-s/go-kryptos"
)

type options[T any] struct {
	keyCodec          codec.KeyCodec
	valueCodec        codec.ValueCodec[T]
	clientOptions     []option.ClientOption
	labels            map[string]string
	ignoreInvalidKeys bool
	clearCredentials  bool
	metrics           kryptos.Metrics
}
