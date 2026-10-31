// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
	operation "github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
)

func (s *Storage[T]) await(ctx context.Context, op *operation.Operation, err error) error {
	if err != nil {
		return err
	}
	if op == nil {
		return kryptos.ErrIndeterminate
	}
	id := op.Id
	delay := operationPollMin
	const maxPollFailures = 3
	failures := 0
	for !op.Done {
		if id == "" {
			return kryptos.ErrIndeterminate
		}
		if waitErr := (kryptos.RealClock{}).Wait(ctx, delay); waitErr != nil {
			return kryptos.ErrIndeterminate
		}
		delay = min(delay*operationPollFactor, time.Second)
		next, pollErr := s.ops.Get(ctx, &operation.GetOperationRequest{OperationId: id})
		if pollErr != nil {
			failures++
			if failures < maxPollFailures && (status.Code(pollErr) == codes.Unavailable || status.Code(pollErr) == codes.DeadlineExceeded) {
				continue
			}
			return kryptos.ErrIndeterminate
		}
		if next == nil {
			return kryptos.ErrIndeterminate
		}
		failures = 0
		op = next
	}
	if e := op.GetError(); e != nil {
		return status.ErrorProto(e)
	}
	return nil
}
func classify(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, kryptos.ErrMalformed) || errors.Is(e, kryptos.ErrIndeterminate) {
		return e
	}
	switch status.Code(e) {
	case codes.PermissionDenied, codes.Unauthenticated:
		return kryptos.ErrPermissionDenied
	case codes.InvalidArgument:
		return kryptos.ErrMalformed
	case codes.FailedPrecondition:
		return kryptos.ErrFailedPrecondition
	case codes.NotFound:
		return kryptos.ErrNotFound
	case codes.AlreadyExists, codes.Aborted:
		return kryptos.ErrConflict
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	default:
		return kryptos.ErrUnavailable
	}
}

const (
	operationPollMin    = 50 * time.Millisecond
	operationPollFactor = 2
)
