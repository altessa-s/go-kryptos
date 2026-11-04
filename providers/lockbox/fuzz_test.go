// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/grpc"

	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
)

func FuzzPayloadDecode(f *testing.F) {
	f.Add(`"value"`)
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 65536 {
			return
		}

		backend := fake.New()

		dial, stop := backend.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
			if _, ok := req.(*pb.GetPayloadRequest); ok {
				return &pb.Payload{Entries: []*pb.Payload_Entry{{Key: "Data", Value: &pb.Payload_Entry_TextValue{TextValue: body}}}}, nil
			}

			return next(ctx, req)
		}))
		defer stop()

		p, e := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))

		require.NoError(t, e)
		defer p.Close(t.Context())

		require.NoError(t, p.Put(t.Context(), "key", "initial"))

		s, e := p.Get(t.Context(), "key")
		if e == nil {
			require.NotNil(t, s)
			require.Equal(t, "key", s.Key)
			s.Clear()
		}
	})
}
