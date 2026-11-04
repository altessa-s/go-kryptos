// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

//go:generate optgen generate --type=tokenOptions --option-type=TokenOption --output=token_options_gen.go
import (
	"time"

	kryptos "github.com/altessa-s/go-kryptos"
)

type tokenOptions struct {
	iamEndpoint   string `opt:"IAMEndpoint" optgen:"default=DefaultIAMEndpoint"`
	clock         kryptos.Clock
	refreshBefore time.Duration `optgen:"default=DefaultRefreshBefore" optval:"nonpositive"`
}

const (
	// DefaultIAMEndpoint is the Yandex Cloud IAM token endpoint.
	DefaultIAMEndpoint = "https://iam.api.cloud.yandex.net/iam/v1/tokens"
	// DefaultRefreshBefore is how long before expiry a token is refreshed.
	DefaultRefreshBefore = time.Minute
)
