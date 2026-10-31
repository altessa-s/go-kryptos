// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
)

type identityCodec struct{ encoded []byte }

func (c *identityCodec) Encode(v []byte) ([]byte, error) { c.encoded = v; return v, nil }
func (*identityCodec) Decode(v []byte) ([]byte, error)   { return v, nil }
func TestPutOwnsIdentityCodecInput(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "backend_error"}[fail], func(t *testing.T) {
			t.Parallel()
			codec := &identityCodec{}
			conn, close, err := fake.New().Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				if fail && strings.HasSuffix(info.FullMethod, "/AddSecretVersion") {
					return nil, status.Error(codes.PermissionDenied, "denied")
				}
				return next(ctx, req)
			}))
			require.NoError(t, err)
			defer close()
			p, err := gcp.New[[]byte](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[[]byte](option.WithGRPCConn(conn)), gcp.WithValueCodec[[]byte](codec))
			require.NoError(t, err)
			defer p.Close(t.Context())
			input := []byte("caller-owned")
			err = p.Put(t.Context(), "key", input)
			if fail {
				require.ErrorIs(t, err, kryptos.ErrPermissionDenied)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, []byte("caller-owned"), input)
			require.Equal(t, make([]byte, len(input)), codec.encoded)
			if !fail {
				got, err := p.Get(t.Context(), "key")
				require.NoError(t, err)
				require.Equal(t, input, got.Value)
				got.Clear()
			}
		})
	}
}
