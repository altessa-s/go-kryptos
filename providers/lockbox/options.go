// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

//go:generate optgen generate
import (
	"github.com/altessa-s/go-kryptos/codec"

	"google.golang.org/grpc"
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
