// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/vault"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestStaleCASRemainsObservable(t *testing.T) {
	t.Parallel()
	b := &backend{data: map[string]*record{"blitz/a2V5": {versions: []string{`"old"`}}}}
	client := newClient(t, b)
	cfg := client.CloneConfig()
	transport := cfg.HttpClient.Transport
	var reads atomic.Int32
	ready := make(chan struct{})
	cfg.HttpClient.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		resp, e := transport.RoundTrip(r)
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/metadata/") {
			n := reads.Add(1)
			if n == 2 {
				close(ready)
			}
			if n <= 2 {
				select {
				case <-ready:
				case <-r.Context().Done():
				}
			}
		}
		return resp, e
	})
	// CloneConfig shares no client struct; construct both providers over the wrapped transport.
	wrapped, e := api.NewClient(cfg)
	require.NoError(t, e)
	a, e := vault.New[string](wrapped, vault.WithCheckAndSet[string]())
	require.NoError(t, e)
	other, e := vault.New[string](wrapped, vault.WithCheckAndSet[string]())
	require.NoError(t, e)
	defer a.Close(context.WithoutCancel(t.Context()))
	defer other.Close(context.WithoutCancel(t.Context()))
	results := make(chan error, 2)
	go func() { results <- a.Put(t.Context(), "key", "first") }()
	go func() { results <- other.Put(t.Context(), "key", "second") }()
	conflicts := 0
	for range 2 {
		e := <-results
		if e != nil {
			require.True(t, errors.Is(e, kryptos.ErrConflict))
			conflicts++
		}
	}
	require.Equal(t, 1, conflicts)
	require.NoError(t, other.Put(t.Context(), "key", "retry"))
	b.mu.Lock()
	require.Len(t, b.data["blitz/a2V5"].versions, 3)
	b.mu.Unlock()
}
