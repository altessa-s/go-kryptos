// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault

//go:generate optgen generate
import "github.com/altessa-s/go-kryptos/codec"

const (
	// DefaultMountPath is the default KV v2 mount.
	DefaultMountPath = "kv"
	// DefaultSecretPath is the default path prefix below the mount.
	DefaultSecretPath = "blitz"
)

type options[T any] struct {
	mountPath               string `optgen:"default=DefaultMountPath"`
	secretPath              string `optgen:"default=DefaultSecretPath"`
	checkAndSet             bool
	destroyPreviousVersions bool
	keyCodec                codec.KeyCodec
	valueCodec              codec.ValueCodec[T]
}
