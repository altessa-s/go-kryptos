// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/vault"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

func TestLostDeleteResponsePreservesRecreation(t *testing.T) {
	t.Parallel()

	for _, transportFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "server-error", true: "transport-error"}[transportFailure], func(t *testing.T) {
			t.Parallel()

			b := &backend{data: map[string]*record{}}

			var (
				deletes  atomic.Int32
				recreate func(context.Context) error
			)

			cfg := api.DefaultConfig()
			cfg.MaxRetries = 3
			cfg.HttpClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
				w := httptest.NewRecorder()
				b.serve(w, r)

				if r.Method == http.MethodDelete {
					deletes.Add(1)

					if err := recreate(r.Context()); err != nil {
						return nil, err
					}

					if transportFailure {
						return nil, errors.New(kryptostest.Canary)
					}

					w = httptest.NewRecorder()
					w.WriteHeader(http.StatusInternalServerError)
				}

				response := w.Result()
				response.Request = r

				return response, nil
			})
			client, err := api.NewClient(cfg)
			require.NoError(t, err)
			p, err := vault.New[string](client)
			require.NoError(t, err)
			other, err := vault.New[string](client)

			require.NoError(t, err)
			defer other.Close(t.Context())

			recreate = func(ctx context.Context) error { return other.Put(ctx, "key", "replacement") }
			m, err := manager.New(p)

			require.NoError(t, err)
			defer m.Close(t.Context())

			require.NoError(t, m.Put(t.Context(), "key", "original"))
			err = m.Delete(t.Context(), "key")
			require.ErrorIs(t, err, kryptos.ErrIndeterminate)
			require.NotContains(t, err.Error(), kryptostest.Canary)
			require.EqualValues(t, 1, deletes.Load())
			got, err := m.Get(t.Context(), "key")
			require.NoError(t, err)
			require.Equal(t, "replacement", got.Value)
			got.Clear()
		})
	}
}

// The SDK does not copy tokens unless CloneToken is enabled. Authenticate the
// transport so this test catches mutations built from an unauthenticated clone.
func TestDeletePreservesClientAuthentication(t *testing.T) {
	t.Parallel()

	b := &backend{data: map[string]*record{}}
	cfg := api.DefaultConfig()
	cfg.MaxRetries = 3
	cfg.CloneToken = false
	cfg.HttpClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		w := httptest.NewRecorder()
		if r.Header.Get("X-Vault-Token") != "test-token" || r.Header.Get("X-Vault-Namespace") != "test-namespace" {
			w.WriteHeader(http.StatusForbidden)
		} else {
			b.serve(w, r)
		}

		response := w.Result()
		response.Request = r

		return response, nil
	})
	client, err := api.NewClient(cfg)
	require.NoError(t, err)
	client.SetToken("test-token")
	client.SetNamespace("test-namespace")
	p, err := vault.New[string](client)

	require.NoError(t, err)
	defer p.Close(t.Context())

	require.NoError(t, p.Put(t.Context(), "key", "value"))
	require.NoError(t, p.Delete(t.Context(), "key"))
	got, err := p.Get(t.Context(), "key")
	require.Nil(t, got)
	require.ErrorIs(t, err, kryptos.ErrNotFound)
	require.Equal(t, 3, client.MaxRetries())
	require.False(t, client.CloneToken())
	require.Equal(t, "test-token", client.Token())
}
