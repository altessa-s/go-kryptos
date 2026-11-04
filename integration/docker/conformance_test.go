// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package docker_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-nomos/loader"
	"github.com/altessa-s/go-nomos/loader/secrets"

	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/memory"
)

var _ secrets.Resolver = manager.StringResolver{}

func TestResolverConformance(t *testing.T) {
	t.Parallel()

	p, e := memory.New(map[string]string{"ns:key": "value"})
	require.NoError(t, e)
	m, e := manager.New(p)

	require.NoError(t, e)
	defer m.Close(t.Context())

	v, e := manager.NewStringResolver(m).Resolve(t.Context(), "ns:key")
	require.NoError(t, e)
	require.Equal(t, "value", v)
	assertBootstrap(t, manager.NewStringResolver(m))
}

func assertBootstrap(t *testing.T, resolver secrets.Resolver) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("token: $__secret{ns:key}\n"), 0600))
	l := loader.New(nil, loader.WithPath(path), loader.WithSkipEnv(), loader.WithSecretResolver(resolver))
	cfg := struct {
		Token string `yaml:"token"`
	}{}
	_, e := l.LoadContext(t.Context(), &cfg)
	require.NoError(t, e)
	require.Equal(t, "value", cfg.Token)
}
