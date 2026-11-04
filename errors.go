// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos

import "errors"

// Domain errors never include secret payloads.
var (
	ErrPermissionDenied        = errors.New("secret access denied")
	ErrFailedPrecondition      = errors.New("secret precondition failed")
	ErrUnavailable             = errors.New("secret backend unavailable")
	ErrNotFound                = errors.New("secret not found")
	ErrInvalidKey              = errors.New("invalid secret key")
	ErrConflict                = errors.New("secret version conflict")
	ErrMalformed               = errors.New("malformed secret")
	ErrClosed                  = errors.New("secret store closed")
	ErrReadOnly                = errors.New("secret store is read only")
	ErrPanicked                = errors.New("secret callback panicked")
	ErrIndeterminate           = errors.New("secret operation outcome indeterminate; do not retry")
	ErrCleanupFailed           = errors.New("secret cleanup failed")
	ErrUncloneablePayload      = errors.New("uncloneable payload")
	ErrKeyEncoding             = errors.New("key encoding failed")
	ErrKeyDecoding             = errors.New("key decoding failed")
	ErrValueEncoding           = errors.New("value encoding failed")
	ErrValueDecoding           = errors.New("value decoding failed")
	ErrFilterRebuildInProgress = errors.New("filter rebuild in progress")
	ErrInvalidOptions          = errors.New("invalid options")
)

// CommittedError reports failure after a write has committed. Do not retry the write.
type CommittedError struct{ Err error }

// Error reports the committed write without the cause.
func (e *CommittedError) Error() string { return "secret committed: post-write operation failed" }

// Unwrap returns the post-write failure.
func (e *CommittedError) Unwrap() error { return e.Err }
