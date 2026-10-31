// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/vault"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

func TestWriteResponseOutcome(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		code       int
		committed  bool
	}{
		{"invalid-version", `{"data":{"version":"invalid"}}`, 200, true},
		{"invalid-json", `{`, 200, true},
		{"empty", ``, 204, true},
		{"lost-response", "", 0, false},
		{"ambiguous-server-error", `{"errors":["failed"]}`, 500, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := &backend{data: map[string]*record{}}
			var fail atomic.Bool
			srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fail.Load() && r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/data/") {
					b.serve(httptest.NewRecorder(), r)
					if tc.code == 0 {
						conn, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = conn.Close()
						return
					}
					w.WriteHeader(tc.code)
					_, _ = w.Write([]byte(tc.body))
					return
				}
				b.serve(w, r)
			}))
			cfg := api.DefaultConfig()
			cfg.HttpClient = srv.Client()
			cfg.Address = srv.URL
			cfg.MaxRetries = 3
			client, err := api.NewClient(cfg)
			require.NoError(t, err)
			p, err := vault.New[string](client)
			require.NoError(t, err)
			require.NoError(t, p.Put(t.Context(), "key", "old"))
			fail.Store(true)
			m, err := manager.New(p)
			require.NoError(t, err)
			defer m.Close(context.WithoutCancel(t.Context()))
			old, err := m.Get(t.Context(), "key")
			require.NoError(t, err)
			old.Clear()
			err = m.Put(t.Context(), "key", "new")
			if tc.committed {
				_, ok := errors.AsType[*kryptos.CommittedError](err)
				require.True(t, ok)
				require.ErrorIs(t, err, kryptos.ErrMalformed)
				got, getErr := m.Get(t.Context(), "key")
				require.NoError(t, getErr)
				require.Equal(t, "new", got.Value)
				got.Clear()
			} else {
				require.ErrorIs(t, err, kryptos.ErrIndeterminate)
			}
			b.mu.Lock()
			writes := b.writes
			b.mu.Unlock()
			require.Equal(t, 2, writes, "one seed and one mutation")
		})
	}
}
