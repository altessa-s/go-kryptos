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
	kryptostest.RunCreationContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string], func(string) int) {
		tb.Helper()
		s := fake.New()
		return provider(tb, s), provider(tb, s), func(key string) int {
			encoded, e := base64key.New().Encode(key)
			require.NoError(tb, e)
			return s.Versions(encoded)
		}
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

func TestListAndLookupRPCCounts(t *testing.T) {
	t.Parallel()
	server := fake.New()
	p := provider(t, server)
	defer p.Close(t.Context())
	require.NoError(t, p.Put(t.Context(), "first", "one"))
	require.NoError(t, p.Put(t.Context(), "second", "two"))
	lists, payloads := server.Counts()
	values, err := p.List(t.Context())
	require.NoError(t, err)
	require.Len(t, values, 2)
	for _, v := range values {
		v.Clear()
	}
	nextLists, nextPayloads := server.Counts()
	require.Equal(t, lists+1, nextLists)
	require.Equal(t, payloads+2, nextPayloads)
	for range 3 {
		v, err := p.Get(t.Context(), "first")
		require.NoError(t, err)
		v.Clear()
		require.NoError(t, p.Put(t.Context(), "first", "new"))
	}
	require.NoError(t, p.Delete(t.Context(), "first"))
	finalLists, _ := server.Counts()
	require.Equal(t, nextLists, finalLists)
}
func TestForeignAndInactiveWriteConflict(t *testing.T) {
	t.Parallel()
	for _, inactive := range []bool{false, true} {
		t.Run(map[bool]string{false: "labels", true: "inactive"}[inactive], func(t *testing.T) {
			t.Parallel()
			server := fake.New()
			owner := provider(t, server, lockbox.WithLabels[string](map[string]string{"owner": "foreign"}))
			defer owner.Close(t.Context())
			require.NoError(t, owner.Put(t.Context(), "key", "old"))
			encoded, err := base64key.New().Encode("key")
			require.NoError(t, err)
			label := "ours"
			if inactive {
				server.Deactivate(encoded)
				label = "foreign"
			}
			other := provider(t, server, lockbox.WithLabels[string](map[string]string{"owner": label}))
			defer other.Close(t.Context())
			require.ErrorIs(t, other.Put(t.Context(), "key", "new"), kryptos.ErrConflict)
			require.Equal(t, 1, server.Versions(encoded))
		})
	}
}
