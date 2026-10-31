// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	"google.golang.org/api/option"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if kryptostest.GoroutineLeaks() != nil {
		code = 1
	}
	os.Exit(code)
}
func provider(t testing.TB, server *fake.Server, opts ...gcp.Option[string]) *gcp.Storage[string] {
	t.Helper()
	conn, close, e := server.Connect()
	require.NoError(t, e)
	t.Cleanup(close)
	p, e := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), append(opts, gcp.WithClientOptions[string](option.WithGRPCConn(conn)))...)
	require.NoError(t, e)
	return p
}
func TestContract(t *testing.T) {
	t.Parallel()
	kryptostest.RunProviderContract(t, func(tb testing.TB) kryptos.Provider[string] { tb.Helper(); return provider(tb, fake.New()) }, kryptostest.Caps{}, "value")
}
func TestCreation(t *testing.T) {
	t.Parallel()
	kryptostest.RunCreationContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
		tb.Helper()
		server := fake.New()
		tb.Cleanup(func() {
			encoded, e := base64key.New().Encode("concurrent")
			require.NoError(tb, e)
			require.Equal(tb, 2, server.Versions("projects/test-project/secrets/"+encoded))
		})
		return provider(tb, server), provider(tb, server)
	}, kryptostest.Caps{}, "value")
}
func TestInvalidCredentials(t *testing.T) {
	t.Parallel()
	for _, b := range [][]byte{nil, []byte(`{}`), []byte(`{"type":"authorized_user"}`)} {
		_, e := gcp.New[string](t.Context(), "test-project", b)
		require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	}
}

func TestPutDelete(t *testing.T) {
	t.Parallel()
	kryptostest.RunPutDeleteContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
		tb.Helper()
		server := fake.New()
		return provider(tb, server), provider(tb, server)
	}, kryptostest.Caps{}, "value")
}
