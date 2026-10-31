// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
)

func typedProvider[T any](tb testing.TB) kryptos.Provider[T] {
	tb.Helper()
	server := fake.New()
	dial, stop := server.Connect()
	tb.Cleanup(stop)
	p, e := lockbox.New[T](tb.Context(), "folder", tokens{}, lockbox.WithDialOptions[T](dial...), lockbox.WithEndpoints[T](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
	require.NoError(tb, e)
	return p
}
func TestPayloadTypes(t *testing.T) {
	t.Parallel()
	t.Run("bytes", func(t *testing.T) {
		t.Parallel()
		kryptostest.RunProviderContract(t, typedProvider[[]byte], kryptostest.Caps{}, []byte("value"))
	})
	t.Run("struct", func(t *testing.T) {
		t.Parallel()
		type payload struct {
			Password string
			Data     []byte
		}
		kryptostest.RunProviderContract(t, typedProvider[payload], kryptostest.Caps{}, payload{Password: "value", Data: []byte("bytes")})
	})
}

func TestManagerContract(t *testing.T) {
	t.Parallel()
	kryptostest.RunManagerContract(t, func(tb testing.TB) kryptostest.Manager[string] {
		tb.Helper()
		m, e := manager.New(typedProvider[string](tb))
		require.NoError(tb, e)
		return m
	}, "value")
}

func TestMalformedBackendPayloads(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"no-version", "metadata-error", "payload-error", "missing-data"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			server := fake.New()
			dial, stop := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				switch req.(type) {
				case *pb.GetSecretRequest:
					if mode == "no-version" {
						return &pb.Secret{}, nil
					}
					if mode == "metadata-error" {
						return nil, status.Error(codes.PermissionDenied, "denied")
					}
				case *pb.GetPayloadRequest:
					if mode == "payload-error" {
						return nil, status.Error(codes.PermissionDenied, "denied")
					}
					if mode == "missing-data" {
						return &pb.Payload{Entries: []*pb.Payload_Entry{{Key: "other"}}}, nil
					}
				}
				return next(ctx, req)
			}))
			defer stop()
			p, e := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
			require.NoError(t, e)
			defer p.Close(t.Context())
			require.NoError(t, p.Put(t.Context(), "key", "value"))
			_, e = p.Get(t.Context(), "key")
			switch mode {
			case "no-version":
				require.ErrorIs(t, e, kryptos.ErrNotFound)
			case "missing-data":
				require.ErrorIs(t, e, kryptos.ErrMalformed)
			default:
				require.ErrorIs(t, e, kryptos.ErrUnavailable)
			}
		})
	}
}
