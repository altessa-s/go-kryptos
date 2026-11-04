// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

func typedProvider[T any](tb testing.TB) kryptos.Provider[T] {
	tb.Helper()

	server := fake.New()
	conn, stop, e := server.Connect()
	require.NoError(tb, e)
	tb.Cleanup(stop)
	p, e := gcp.New[T](tb.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[T](option.WithGRPCConn(conn)))
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
