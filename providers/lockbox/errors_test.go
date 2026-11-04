// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

type rawKey struct{ fail bool }

func (c rawKey) Encode(v string) (string, error) {
	if c.fail {
		return "", errors.New(kryptostest.Canary)
	}
	return v, nil
}
func (c rawKey) Decode(v string) (string, error) {
	if c.fail {
		return "", errors.New(kryptostest.Canary)
	}
	return v, nil
}

type badValue struct{}

func (badValue) Encode(string) ([]byte, error) { return nil, errors.New(kryptostest.Canary) }
func (badValue) Decode([]byte) (string, error) { return "", errors.New(kryptostest.Canary) }
func TestCodecFailuresAndInvalidListing(t *testing.T) {
	t.Parallel()
	server := fake.New()
	plain := provider(t, server, lockbox.WithKeyCodec[string](rawKey{}), lockbox.WithLabels[string](map[string]string{"env": "test"}))
	require.NoError(t, plain.Put(t.Context(), "raw-key", "value"))
	defer plain.Close(t.Context())
	p := provider(t, server)
	defer p.Close(t.Context())
	_, e := p.List(t.Context())
	require.ErrorIs(t, e, kryptos.ErrKeyDecoding)
	skip := provider(t, server, lockbox.WithIgnoreInvalidKeys[string]())
	defer skip.Close(t.Context())
	all, e := skip.List(t.Context())
	require.NoError(t, e)
	require.Empty(t, all)
	bad := provider(t, server, lockbox.WithValueCodec[string](badValue{}))
	defer bad.Close(t.Context())
	require.ErrorIs(t, bad.Put(t.Context(), "key", "value"), kryptos.ErrValueEncoding)
	require.NoError(t, p.Put(t.Context(), "key", "value"))
	_, e = bad.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrValueDecoding)
	badKey := provider(t, server, lockbox.WithKeyCodec[string](rawKey{fail: true}))
	defer badKey.Close(t.Context())
	_, e = badKey.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrKeyEncoding)
}
func TestRPCClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code codes.Code
		want error
	}{{codes.Unauthenticated, kryptos.ErrPermissionDenied}, {codes.InvalidArgument, kryptos.ErrMalformed}, {codes.FailedPrecondition, kryptos.ErrFailedPrecondition}, {codes.Aborted, kryptos.ErrConflict}, {codes.PermissionDenied, kryptos.ErrPermissionDenied}, {codes.Canceled, context.Canceled}, {codes.DeadlineExceeded, context.DeadlineExceeded}} {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()
			server := fake.New()
			dial, close := server.Connect(grpc.UnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
				return nil, status.Error(tc.code, kryptostest.Canary)
			}))
			defer close()
			p, e := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
			require.NoError(t, e)
			defer p.Close(t.Context())
			require.ErrorIs(t, p.Ping(t.Context()), tc.want)
			_, e = p.Get(t.Context(), "key")
			require.ErrorIs(t, e, tc.want)
			require.NotContains(t, e.Error(), kryptostest.Canary)
		})
	}
}
