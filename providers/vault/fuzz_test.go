// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/vault"

	api "github.com/hashicorp/vault/api"
)

func FuzzKVExtraction(f *testing.F) {
	f.Add(`{"data":{"data":{"data":"InZhbHVlIg=="},"metadata":{"version":1}}}`)
	f.Add(`{}`)
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 65536 {
			return
		}
		srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(body))
		}))
		cfg := api.DefaultConfig()
		cfg.HttpClient = srv.Client()
		cfg.Address = srv.URL
		cfg.MaxRetries = 0
		c, e := api.NewClient(cfg)
		require.NoError(t, e)
		p, e := vault.New[string](c)
		require.NoError(t, e)
		defer p.Close(t.Context())
		s, e := p.Get(t.Context(), "key")
		if e == nil {
			require.NotNil(t, s)
			require.Equal(t, "key", s.Key)
			s.Clear()
		}
	})
}
