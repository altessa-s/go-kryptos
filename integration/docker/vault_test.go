// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

//go:build integration_docker

package docker_test

import (
	"context"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/vault"

	httpclient "github.com/altessa-s/go-atlas-net/http/client"
	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
	tcvault "github.com/testcontainers/testcontainers-go/modules/vault"
)

func TestVaultKV(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	t.Parallel()
	ctx := t.Context()
	ctr, e := tcvault.Run(ctx, "hashicorp/vault:1.20.0", tcvault.WithToken("integration-root-token"))
	testcontainers.CleanupContainer(t, ctr)
	require.NoError(t, e)
	addr, e := ctr.HttpHostAddress(ctx)
	require.NoError(t, e)
	cfg := api.DefaultConfig()
	cfg.Address = addr
	cfg.HttpClient = httpclient.New(httpclient.WithSSRFAllowedCIDRs(netip.MustParsePrefix("127.0.0.0/8")), httpclient.WithRetryMax(0))
	client, e := api.NewClient(cfg)
	require.NoError(t, e)
	client.SetToken("integration-root-token")
	require.NoError(t, client.Sys().MountWithContext(ctx, "kv", &api.MountInput{Type: "kv", Options: map[string]string{"version": "2"}}))
	kryptostest.RunProviderContract(t, func(tb testing.TB) kryptos.Provider[string] {
		tb.Helper()
		p, e := vault.New[string](client, vault.WithSecretPath[string]("contract"))
		require.NoError(tb, e)
		return p
	}, kryptostest.Caps{}, "value")
	kryptostest.RunCreationContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
		tb.Helper()
		a, e := vault.New[string](client, vault.WithSecretPath[string]("concurrent"), vault.WithCheckAndSet[string]())
		require.NoError(tb, e)
		b, e := vault.New[string](client, vault.WithSecretPath[string]("concurrent"), vault.WithCheckAndSet[string]())
		require.NoError(tb, e)
		return a, b
	}, kryptostest.Caps{WritePolicy: kryptostest.CheckAndSet}, "value")
	require.NoError(t, client.Sys().PutPolicyWithContext(ctx, "narrow", `path "kv/data/narrow/*" { capabilities = ["create", "read", "update", "delete"] }
path "kv/metadata/narrow/*" { capabilities = ["read", "list", "delete"] }
path "auth/token/lookup-self" { capabilities = ["read"] }`))
	token, e := client.Auth().Token().CreateWithContext(ctx, &api.TokenCreateRequest{Policies: []string{"narrow"}})
	require.NoError(t, e)
	limited, e := api.NewClient(cfg)
	require.NoError(t, e)
	limited.SetToken(token.Auth.ClientToken)
	p, e := vault.New[string](limited, vault.WithSecretPath[string]("narrow"), vault.WithDestroyPreviousVersions[string]())
	require.NoError(t, e)
	m, e := manager.New(p)
	require.NoError(t, e)
	defer m.Close(context.WithoutCancel(ctx))
	require.NoError(t, p.Ping(ctx))
	require.NoError(t, m.Put(ctx, "key", "old"))
	require.ErrorIs(t, m.Put(ctx, "key", "new"), kryptos.ErrCleanupFailed)
	v, e := m.Get(ctx, "key")
	require.NoError(t, e)
	require.Equal(t, "new", v.Value)
	v.Clear()
	require.NoError(t, m.Put(ctx, "ns:key", "value"))
	assertBootstrap(t, manager.NewStringResolver(m))
}
