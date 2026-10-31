// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/vault"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

type record struct {
	versions []string
	deleted  bool
}
type backend struct {
	mu          sync.Mutex
	data        map[string]*record
	denyDestroy bool
	writes      int
}

func newClient(t testing.TB, b *backend) *api.Client {
	t.Helper()
	srv := httptest.NewTestServer(t, http.HandlerFunc(b.serve))
	cfg := api.DefaultConfig()
	cfg.HttpClient = srv.Client()
	cfg.Address = srv.URL
	cfg.MaxRetries = 0
	c, e := api.NewClient(cfg)
	require.NoError(t, e)
	return c
}
func (b *backend) serve(w http.ResponseWriter, r *http.Request) {
	b.mu.Lock()
	defer b.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	fail := func(code int, msg string) { w.WriteHeader(code); write(map[string]any{"errors": []string{msg}}) }
	if r.URL.Path == "/v1/auth/token/lookup-self" {
		write(map[string]any{"data": map[string]any{"id": "test"}})
		return
	}
	p := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/v1/kv/"), "/", 2)
	if len(p) != 2 {
		fail(404, "not found")
		return
	}
	kind, key := p[0], p[1]
	v := b.data[key]
	if kind == "metadata" && (r.Method == "LIST" || r.URL.Query().Get("list") == "true") {
		var keys []string
		for k := range b.data {
			if after, ok := strings.CutPrefix(k, key+"/"); ok {
				keys = append(keys, after)
			}
		}
		write(map[string]any{"data": map[string]any{"keys": keys}})
		return
	}
	switch kind {
	case "data":
		if r.Method == http.MethodGet {
			if v == nil || v.deleted {
				fail(404, "not found")
				return
			}
			write(map[string]any{"data": map[string]any{"data": map[string]any{"data": v.versions[len(v.versions)-1]}, "metadata": map[string]any{"version": len(v.versions)}}})
			return
		}
		var body struct {
			Data    map[string]string `json:"data"`
			Options map[string]int    `json:"options"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			fail(400, "bad body")
			return
		}
		current := 0
		if v != nil {
			current = len(v.versions)
		}
		if cas, ok := body.Options["cas"]; ok && cas != current {
			fail(400, "check-and-set mismatch")
			return
		}
		if v == nil {
			v = &record{}
			b.data[key] = v
		}
		v.versions = append(v.versions, body.Data["data"])
		v.deleted = false
		b.writes++
		write(map[string]any{"data": map[string]any{"version": len(v.versions)}})
	case "metadata":
		if v == nil {
			fail(404, "not found")
			return
		}
		if r.Method == http.MethodDelete {
			delete(b.data, key)
			w.WriteHeader(204)
			return
		}
		versions := map[string]any{}
		for i := range v.versions {
			versions[strconv.Itoa(i+1)] = map[string]any{"destroyed": false}
		}
		write(map[string]any{"data": map[string]any{"current_version": len(v.versions), "versions": versions}})
	case "destroy":
		if b.denyDestroy {
			fail(403, "denied")
			return
		}
		w.WriteHeader(204)
	default:
		fail(404, "not found")
	}
}
func TestMain(m *testing.M) {
	code := m.Run()
	if kryptostest.GoroutineLeaks() != nil {
		code = 1
	}
	os.Exit(code)
}
func TestContract(t *testing.T) {
	t.Parallel()
	kryptostest.RunProviderContract(t, func(tb testing.TB) kryptos.Provider[string] {
		tb.Helper()
		c := newClient(tb, &backend{data: map[string]*record{}})
		p, e := vault.New[string](c)
		require.NoError(tb, e)
		return p
	}, kryptostest.Caps{}, "value")
}
func TestCreation(t *testing.T) {
	t.Parallel()
	for _, cas := range []bool{false, true} {
		t.Run(fmt.Sprint(cas), func(t *testing.T) {
			t.Parallel()
			caps := kryptostest.Caps{}
			if cas {
				caps.WritePolicy = kryptostest.CheckAndSet
			}
			kryptostest.RunCreationContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string], func(string) int) {
				tb.Helper()
				b := &backend{data: map[string]*record{}}
				c := newClient(tb, b)
				var opts []vault.Option[string]
				if cas {
					opts = append(opts, vault.WithCheckAndSet[string]())
				}
				a, e := vault.New[string](c, opts...)
				require.NoError(tb, e)
				other, e := vault.New[string](c, opts...)
				require.NoError(tb, e)
				return a, other, func(key string) int {
					value, e := a.Get(tb.Context(), key)
					require.NoError(tb, e)
					defer value.Clear()
					count, e := strconv.Atoi(value.Version)
					require.NoError(tb, e)
					return count
				}
			}, caps, "value")
		})
	}
}
func TestCommittedDestroyDenied(t *testing.T) {
	t.Parallel()
	b := &backend{data: map[string]*record{}, denyDestroy: true}
	p, e := vault.New[string](newClient(t, b), vault.WithDestroyPreviousVersions[string]())
	require.NoError(t, e)
	m, e := manager.New(p)
	require.NoError(t, e)
	defer m.Close(context.WithoutCancel(t.Context()))
	require.NoError(t, m.Put(t.Context(), "key", "old"))
	s, e := m.Watch(t.Context(), manager.WithSkipInitial())
	require.NoError(t, e)
	defer s.Close()
	require.ErrorIs(t, m.Put(t.Context(), "key", "new"), kryptos.ErrCleanupFailed)
	event := <-s.Events()
	require.Equal(t, kryptos.Updated, event.Type)
	event.Clear()
	v, e := m.Get(t.Context(), "key")
	require.NoError(t, e)
	require.Equal(t, "new", v.Value)
	v.Clear()
	b.mu.Lock()
	require.Equal(t, 2, b.writes)
	b.mu.Unlock()
}
func TestInvalidConstructor(t *testing.T) {
	t.Parallel()
	_, e := vault.New[string](nil)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
}

func TestPutDelete(t *testing.T) {
	t.Parallel()
	for _, cas := range []bool{false, true} {
		t.Run(fmt.Sprint(cas), func(t *testing.T) {
			t.Parallel()
			caps := kryptostest.Caps{}
			if cas {
				caps.WritePolicy = kryptostest.CheckAndSet
			}
			kryptostest.RunPutDeleteContract(t, func(tb testing.TB) (kryptos.Provider[string], kryptos.Provider[string]) {
				tb.Helper()
				b := &backend{data: map[string]*record{}}
				c := newClient(tb, b)
				var opts []vault.Option[string]
				if cas {
					opts = append(opts, vault.WithCheckAndSet[string]())
				}
				a, e := vault.New[string](c, opts...)
				require.NoError(tb, e)
				other, e := vault.New[string](c, opts...)
				require.NoError(tb, e)
				return a, other
			}, caps, "value")
		})
	}
}
