// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"

	kryptos "github.com/altessa-s/go-kryptos"
	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
)

func TestRenameBetweenListAndGet(t *testing.T) {
	t.Parallel()
	for _, action := range []string{"get", "put", "delete"} {
		t.Run(action, func(t *testing.T) {
			t.Parallel()
			server := fake.New()
			dial, stop := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				result, err := next(ctx, req)
				if _, ok := req.(*pb.GetSecretRequest); ok && err == nil {
					result.(*pb.Secret).Name = "renamed"
				}
				return result, err
			}))
			defer stop()
			p, err := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
			require.NoError(t, err)
			defer p.Close(t.Context())
			require.NoError(t, p.Put(t.Context(), "key", "original"))
			for range 2 {
				switch action {
				case "get":
					_, err = p.Get(t.Context(), "key")
				case "put":
					err = p.Put(t.Context(), "key", "replacement")
				case "delete":
					err = p.Delete(t.Context(), "key")
				}
				require.ErrorIs(t, err, kryptos.ErrConflict)
			}
			lists, reads := server.Counts()
			require.Equal(t, 3, lists, "each rejected lookup invalidates the cached mapping")
			require.Zero(t, reads)
			require.Equal(t, 1, server.Versions("a2V5"))
		})
	}
}
