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

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

type identityCodec struct{ encoded []byte }

func (c *identityCodec) Encode(v []byte) ([]byte, error) { c.encoded = v; return v, nil }
func (*identityCodec) Decode(v []byte) ([]byte, error)   { return v, nil }
func TestPutOwnsIdentityCodecInput(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "backend_error"}[fail], func(t *testing.T) {
			t.Parallel()
			codec := &identityCodec{}
			b := &backend{data: map[string]*record{}}
			srv := httptest.NewTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if fail {
					w.WriteHeader(http.StatusForbidden)
					return
				}
				b.serve(w, r)
			}))
			cfg := api.DefaultConfig()
			cfg.HttpClient = srv.Client()
			cfg.Address = srv.URL
			cfg.MaxRetries = 0
			client, err := api.NewClient(cfg)
			require.NoError(t, err)
			p, err := vault.New[[]byte](client, vault.WithValueCodec[[]byte](codec))
			require.NoError(t, err)
			defer p.Close(t.Context())
			input := []byte("caller-owned")
			err = p.Put(t.Context(), "key", input)
			if fail {
				require.ErrorIs(t, err, kryptos.ErrPermissionDenied)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, []byte("caller-owned"), input)
			require.Equal(t, make([]byte, len(input)), codec.encoded)
			if !fail {
				got, err := p.Get(t.Context(), "key")
				require.NoError(t, err)
				require.Equal(t, input, got.Value)
				got.Clear()
			}
		})
	}
}
