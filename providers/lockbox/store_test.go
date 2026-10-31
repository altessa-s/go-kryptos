// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

type tokens struct{}

func (tokens) Token(context.Context) (string, error) { return "test", nil }
func TestMain(m *testing.M) {
	code := m.Run()
	if kryptostest.GoroutineLeaks() != nil {
		code = 1
	}
	os.Exit(code)
}
func provider(t testing.TB, s *fake.Server, extra ...lockbox.Option[string]) *lockbox.Storage[string] {
	t.Helper()
	opts, close := s.Connect()
	t.Cleanup(close)
	p, e := lockbox.New[string](t.Context(), "folder", tokens{}, append(extra, lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithDialOptions[string](opts...))...)
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
		s := fake.New()
		tb.Cleanup(func() {
			encoded, e := base64key.New().Encode("concurrent")
			require.NoError(tb, e)
			require.Equal(tb, 2, s.Versions(encoded))
		})
		return provider(tb, s), provider(tb, s)
	}, kryptostest.Caps{}, "value")
}
func TestInvalidConstructor(t *testing.T) {
	t.Parallel()
	_, e := lockbox.New[string](t.Context(), "", tokens{})
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
}

func TestPutDelete(t *testing.T) {
	t.Parallel()
	kryptostest.RunPutDeleteContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
		tb.Helper()
		s := fake.New()
		return provider(tb, s), provider(tb, s)
	}, kryptostest.Caps{}, "value")
}
