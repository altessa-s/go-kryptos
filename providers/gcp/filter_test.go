// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"
	"google.golang.org/grpc"

	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	pb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

func FuzzLabelFilter(f *testing.F) {
	f.Add("environment", "test")
	f.Add("x OR y", "bad")
	f.Fuzz(func(t *testing.T, k, v string) {
		server := fake.New()
		conn, stop, e := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if r, ok := req.(*pb.ListSecretsRequest); ok {
				require.Equal(t, "labels."+k+"="+v, r.Filter)
				require.False(t, strings.ContainsAny(r.Filter, "\"'\n\r"))
				require.NotContains(t, r.Filter, " OR ")
				require.NotContains(t, r.Filter, " AND ")
			}

			return next(ctx, req)
		}))
		require.NoError(t, e)

		defer stop()

		p, e := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithLabels[string](map[string]string{k: v}), gcp.WithClientOptions[string](option.WithGRPCConn(conn)))
		if e != nil {
			return
		}
		defer p.Close(t.Context())

		_, e = p.List(t.Context())
		require.NoError(t, e)
	})
}
