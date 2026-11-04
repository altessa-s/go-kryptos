// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"
	"strings"

	"github.com/altessa-s/go-kryptos"
)

// StringResolver resolves namespace:key references through a Getter.
type StringResolver struct{ getter kryptos.Getter[string] }

// NewStringResolver returns a resolver reading through getter.
func NewStringResolver(getter kryptos.Getter[string]) StringResolver {
	return StringResolver{getter: getter}
}

// Resolve returns a copy of the value of ref and clears the fetched secret.
func (r StringResolver) Resolve(ctx context.Context, ref string) (string, error) {
	if r.getter == nil {
		return "", kryptos.ErrInvalidOptions
	}

	s, e := r.getter.Get(ctx, ref)
	if e != nil {
		return "", e
	}
	defer s.Clear()

	return strings.Clone(s.Value), nil
}
