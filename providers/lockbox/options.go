// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

//go:generate optgen generate
import (
	"google.golang.org/grpc"

	"github.com/altessa-s/go-kryptos/codec"
)

// Endpoints identifies the three Lockbox services.
type Endpoints struct {
	Secret    string
	Payload   string
	Operation string
}
type options[T any] struct {
	keyCodec          codec.KeyCodec
	valueCodec        codec.ValueCodec[T]
	endpoints         Endpoints
	dialOptions       []grpc.DialOption
	labels            map[string]string
	ignoreInvalidKeys bool
}
