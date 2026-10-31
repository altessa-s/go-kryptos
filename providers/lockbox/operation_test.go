// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"

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
