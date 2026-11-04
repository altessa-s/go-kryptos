// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestLostMutationResponseDoesNotReplay(t *testing.T) {
	t.Parallel()
	for _, code := range []codes.Code{codes.Unavailable, codes.DeadlineExceeded, codes.Canceled, codes.Unknown, codes.Internal} {
		t.Run(code.String(), func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				server := fake.New()
				var lose atomic.Bool
				var mutations atomic.Int32
				intercept := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
					reply, err := next(ctx, req)
					if err == nil && strings.HasSuffix(info.FullMethod, "/AddSecretVersion") {
						mutations.Add(1)
						if lose.Load() {
							return nil, status.Error(code, kryptostest.Canary)
						}
					}
					return reply, err
				}
				conn, closeServer, err := server.Connect(grpc.UnaryInterceptor(intercept))
				require.NoError(t, err)
				defer closeServer()
				p, err := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithGRPCConn(conn)))
				require.NoError(t, err)
				m, err := manager.New(p, manager.WithMaxRetries[string](3))
				require.NoError(t, err)
				defer m.Close(t.Context())
				encoded, err := base64key.New().Encode("key")
				require.NoError(t, err)
				// First write exercises creation; the next write exercises an existing key.
				for i, value := range []string{"first", "second"} {
					lose.Store(true)
					err = m.Put(t.Context(), "key", value)
					require.ErrorIs(t, err, kryptos.ErrIndeterminate)
					require.NotErrorIs(t, err, kryptos.ErrUnavailable)
					require.NotContains(t, err.Error(), kryptostest.Canary)
					require.EqualValues(t, i+1, mutations.Load())
					require.Equal(t, i+1, server.Versions("projects/test-project/secrets/"+encoded))
					cached, cacheErr := m.Cached("key")
					require.Nil(t, cached)
					require.ErrorIs(t, cacheErr, kryptos.ErrNotFound)
					lose.Store(false)
					fresh, readErr := m.Get(t.Context(), "key")
					require.NoError(t, readErr)
					require.Equal(t, value, fresh.Value)
					fresh.Clear()
				}
			})
		})
	}
}

func TestMutationDefinitiveRejection(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		code codes.Code
		want error
	}{
		{codes.PermissionDenied, kryptos.ErrPermissionDenied},
		{codes.Unauthenticated, kryptos.ErrPermissionDenied},
		{codes.InvalidArgument, kryptos.ErrMalformed},
		{codes.FailedPrecondition, kryptos.ErrFailedPrecondition},
		{codes.NotFound, kryptos.ErrNotFound},
		{codes.Aborted, kryptos.ErrConflict},
	} {
		t.Run(tc.code.String(), func(t *testing.T) {
			t.Parallel()
			server := fake.New()
			intercept := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				if strings.HasSuffix(info.FullMethod, "/AddSecretVersion") {
					return nil, status.Error(tc.code, kryptostest.Canary)
				}
				return next(ctx, req)
			}
			conn, closeServer, err := server.Connect(grpc.UnaryInterceptor(intercept))
			require.NoError(t, err)
			defer closeServer()
			p, err := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithGRPCConn(conn)))
			require.NoError(t, err)
			defer p.Close(t.Context())
			err = p.Put(t.Context(), "key", "value")
			require.ErrorIs(t, err, tc.want)
			require.NotErrorIs(t, err, kryptos.ErrIndeterminate)
			require.NotContains(t, err.Error(), kryptostest.Canary)
		})
	}
}
