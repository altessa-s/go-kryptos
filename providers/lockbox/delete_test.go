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

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestLostDeleteResponsePreservesRecreation(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		server := fake.New()
		var deletes atomic.Int32
		var recreate func(context.Context) error
		intercept := func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			reply, err := next(ctx, req)
			if err == nil && strings.HasSuffix(info.FullMethod, "/Delete") {
				deletes.Add(1)
				if err := recreate(ctx); err != nil {
					return nil, err
				}
				return nil, status.Error(codes.Unavailable, kryptostest.Canary)
			}
			return reply, err
		}

		dial, closeServer := server.Connect(grpc.UnaryInterceptor(intercept))
		defer closeServer()
		newProvider := func() *lockbox.Storage[string] {
			p, err := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))
			require.NoError(t, err)
			return p
		}

		p, other := newProvider(), newProvider()
		defer other.Close(t.Context())
		recreate = func(ctx context.Context) error { return other.Put(ctx, "key", "replacement") }
		m, err := manager.New(p, manager.WithMaxRetries[string](3))
		require.NoError(t, err)
		defer m.Close(t.Context())
		require.NoError(t, m.Put(t.Context(), "key", "original"))
		err = m.Delete(t.Context(), "key")
		require.ErrorIs(t, err, kryptos.ErrIndeterminate)
		require.NotContains(t, err.Error(), kryptostest.Canary)
		require.EqualValues(t, 1, deletes.Load())
		got, err := m.Get(t.Context(), "key")
		require.NoError(t, err)
		require.Equal(t, "replacement", got.Value)
		got.Clear()
	})
}
