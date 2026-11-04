// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
)

// classifyMutation preserves definitive rejections. Other RPC failures may hide
// a committed mutation, so replaying the request could create another version.
func classifyMutation(err error) error {
	if err == nil {
		return nil
	}

	switch status.Code(err) {
	case codes.PermissionDenied, codes.Unauthenticated, codes.InvalidArgument,
		codes.FailedPrecondition, codes.NotFound, codes.AlreadyExists, codes.Aborted,
		codes.ResourceExhausted, codes.Unimplemented, codes.OutOfRange:
		return classify(err)
	default:
		return kryptos.ErrIndeterminate
	}
}
