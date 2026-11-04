// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

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
	plain := provider(t, server, gcp.WithKeyCodec[string](rawKey{}))

	require.NoError(t, plain.Put(t.Context(), "raw-key", "value"))
	defer plain.Close(t.Context())

	p := provider(t, server)
	defer p.Close(t.Context())

	_, e := p.List(t.Context())
	require.ErrorIs(t, e, kryptos.ErrKeyDecoding)

	skip := provider(t, server, gcp.WithIgnoreInvalidKeys[string]())
	defer skip.Close(t.Context())

	all, e := skip.List(t.Context())
	require.NoError(t, e)
	require.Empty(t, all)

	bad := provider(t, server, gcp.WithValueCodec[string](badValue{}))
	defer bad.Close(t.Context())

	require.ErrorIs(t, bad.Put(t.Context(), "key", "value"), kryptos.ErrValueEncoding)
	require.NoError(t, p.Put(t.Context(), "key", "value"))
	_, e = bad.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrValueDecoding)

	badKey := provider(t, server, gcp.WithKeyCodec[string](rawKey{fail: true}))
	defer badKey.Close(t.Context())

	_, e = badKey.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrKeyEncoding)
}
func TestLabelsAndCredentialOwnership(t *testing.T) {
	t.Parallel()

	server := fake.New()
	conn, stop, e := server.Connect()
	require.NoError(t, e)

	defer stop()

	creds := []byte(`{"type":"service_account"}`)
	p, e := gcp.New[string](t.Context(), "test-project", creds, gcp.WithClearCredentials[string](), gcp.WithLabels[string](map[string]string{"env": "test", "app": "secrets"}), gcp.WithClientOptions[string](option.WithGRPCConn(conn)))

	require.NoError(t, e)
	defer p.Close(t.Context())

	require.Equal(t, make([]byte, len(creds)), creds)
	_, e = p.List(t.Context())
	require.NoError(t, e)

	for _, labels := range []map[string]string{{"bad OR key": "x"}, {"valid": strings.Repeat("x", 64)}, {"x": "A"}} {
		_, e := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithLabels[string](labels))
		require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	}

	_, e = gcp.New[string](t.Context(), "bad", nil)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
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
			conn, stop, e := server.Connect(grpc.UnaryInterceptor(func(context.Context, any, *grpc.UnaryServerInfo, grpc.UnaryHandler) (any, error) {
				return nil, status.Error(tc.code, kryptostest.Canary)
			}))
			require.NoError(t, e)

			defer stop()

			p, e := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithGRPCConn(conn)))

			require.NoError(t, e)
			defer p.Close(t.Context())

			require.ErrorIs(t, p.Ping(t.Context()), tc.want)
			_, e = p.Get(t.Context(), "key")
			require.ErrorIs(t, e, tc.want)
			require.NotContains(t, e.Error(), kryptostest.Canary)
		})
	}
}

type skipMetrics struct{ skipped atomic.Int32 }

func (*skipMetrics) CacheHit()                    {}
func (*skipMetrics) CacheMiss()                   {}
func (*skipMetrics) FilterFailed()                {}
func (*skipMetrics) Fetched(time.Duration, error) {}
func (*skipMetrics) Cycled(time.Duration, error)  {}
func (*skipMetrics) CacheSize(int)                {}
func (m *skipMetrics) Skipped()                   { m.skipped.Add(1) }
func TestListSkipsDisabledLatestVersion(t *testing.T) {
	t.Parallel()

	server := fake.New()
	conn, closeServer, err := server.Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
		if strings.HasSuffix(info.FullMethod, "/AccessSecretVersion") {
			return nil, status.Error(codes.FailedPrecondition, "disabled")
		}

		return next(ctx, req)
	}))
	require.NoError(t, err)

	defer closeServer()

	metrics := &skipMetrics{}
	p, err := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithGRPCConn(conn)), gcp.WithMetrics[string](metrics))

	require.NoError(t, err)
	defer p.Close(t.Context())

	require.NoError(t, p.Put(t.Context(), "key", "value"))
	values, err := p.List(t.Context())
	require.NoError(t, err)
	require.Empty(t, values)
	require.EqualValues(t, 1, metrics.skipped.Load())
}
