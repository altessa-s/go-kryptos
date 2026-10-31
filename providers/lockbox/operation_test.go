// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
	operation "github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
)

func TestPendingOperation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		server := fake.New()
		dial, close := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			reply, e := next(ctx, req)
			if e == nil && strings.HasSuffix(info.FullMethod, "/Create") {
				reply.(*operation.Operation).Done = false
			}
			return reply, e
		}))
		defer close()
		p, e := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
		require.NoError(t, e)
		defer p.Close(t.Context())
		require.NoError(t, p.Put(t.Context(), "key", "value"))
	})
}

func TestPollingOutageDoesNotReplayMutation(t *testing.T) {
	t.Parallel()
	for _, permanent := range []bool{false, true} {
		t.Run(map[bool]string{false: "transient", true: "indeterminate"}[permanent], func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				server := fake.New()
				var polls atomic.Int32
				var writes atomic.Int32
				dial, close := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
					if r, ok := req.(*operation.GetOperationRequest); ok {
						if r.OperationId != "create" && r.OperationId != "add" {
							return nil, status.Error(codes.InvalidArgument, "wrong operation")
						}
						n := polls.Add(1)
						if permanent || n%2 == 1 {
							return nil, status.Error(codes.Unavailable, "poll outage")
						}
					}
					reply, err := next(ctx, req)
					if err == nil && (strings.HasSuffix(info.FullMethod, "/Create") || strings.HasSuffix(info.FullMethod, "/AddVersion")) {
						writes.Add(1)
						reply.(*operation.Operation).Done = false
					}
					return reply, err
				}))
				defer close()
				p, err := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
				require.NoError(t, err)
				m, err := manager.New(p)
				require.NoError(t, err)
				defer m.Close(t.Context())
				for i := 1; i <= 2; i++ {
					err = m.Put(t.Context(), "key", "value")
					if permanent {
						require.ErrorIs(t, err, kryptos.ErrIndeterminate)
						require.NotErrorIs(t, err, kryptos.ErrUnavailable)
					} else {
						require.NoError(t, err)
					}
					encoded, err := base64key.New().Encode("key")
					require.NoError(t, err)
					require.Equal(t, i, server.Versions(encoded))
					require.EqualValues(t, i, writes.Load())
				}
				if permanent {
					require.EqualValues(t, 6, polls.Load())
				} else {
					require.EqualValues(t, 4, polls.Load())
				}
			})
		})
	}
}
