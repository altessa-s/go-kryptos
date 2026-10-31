// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/vault"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

type brokenCodec struct{ panics bool }

func (c brokenCodec) Encode(string) ([]byte, error) { return nil, errors.New(kryptostest.Canary) }
func (c brokenCodec) Decode([]byte) (string, error) {
	if c.panics {
		panic(kryptostest.Canary)
	}
	return "", errors.New(kryptostest.Canary)
}

type brokenKey struct{}

func (brokenKey) Encode(string) (string, error) { return "", errors.New(kryptostest.Canary) }
func (brokenKey) Decode(string) (string, error) { return "", errors.New(kryptostest.Canary) }
func TestDecoderPanicCanary(t *testing.T) {
	t.Parallel()
	b := &backend{data: map[string]*record{}}
	c := newClient(t, b)
	writer, e := vault.New[string](c)
	require.NoError(t, e)
	require.NoError(t, writer.Put(t.Context(), "key", "value"))
	reader, e := vault.New[string](c, vault.WithValueCodec[string](brokenCodec{panics: true}))
	require.NoError(t, e)
	var logs bytes.Buffer
	m, e := manager.New(reader, manager.WithLogger[string](slog.New(slog.NewTextHandler(&logs, nil))))
	require.NoError(t, e)
	defer m.Close(context.WithoutCancel(t.Context()))
	_, e = m.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrPanicked)
	require.NotContains(t, e.Error(), kryptostest.Canary)
	require.NotContains(t, logs.String(), kryptostest.Canary)
}
func TestErrorsAreSanitized(t *testing.T) {
	t.Parallel()
	c := newClient(t, &backend{data: map[string]*record{}})
	p, e := vault.New[string](c, vault.WithValueCodec[string](brokenCodec{}))
	require.NoError(t, e)
	require.ErrorIs(t, p.Put(t.Context(), "key", "value"), kryptos.ErrValueEncoding)
	p, e = vault.New[string](c, vault.WithKeyCodec[string](brokenKey{}))
	require.NoError(t, e)
	_, e = p.Get(t.Context(), "key")
	require.ErrorIs(t, e, kryptos.ErrKeyEncoding)
	for _, opt := range []vault.Option[string]{vault.WithMountPath[string](".."), vault.WithSecretPath[string]("/escape")} {
		_, e = vault.New[string](c, opt)
		require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	}
}
func TestMalformedAndConflict(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		code       int
		want       error
	}{{"malformed", `{"data":{"data":{"unexpected":"x"},"metadata":{"version":1}}}`, 200, kryptos.ErrMalformed}, {"cas", `{"errors":["check-and-set mismatch"]}`, 400, kryptos.ErrConflict}, {"denied", `{"errors":["` + kryptostest.Canary + `"]}`, 403, kryptos.ErrUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.code)
				_, _ = w.Write([]byte(tc.body))
			}))
			cfg := api.DefaultConfig()
			cfg.HttpClient = srv.Client()
			cfg.Address = srv.URL
			cfg.MaxRetries = 0
			c, e := api.NewClient(cfg)
			require.NoError(t, e)
			p, e := vault.New[string](c)
			require.NoError(t, e)
			_, e = p.Get(t.Context(), "key")
			require.ErrorIs(t, e, tc.want)
			require.NotContains(t, e.Error(), kryptostest.Canary)
		})
	}
}
