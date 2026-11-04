// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

//go:build integration

package lockbox_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"
)

func TestIntegrationTLS(t *testing.T) {
	t.Parallel()

	server := fake.New()

	dial, stop, e := server.ConnectTCP(t.Context())
	if e != nil {
		t.Skipf("TCP listener unavailable: %v", e)
	}
	defer stop()

	p, e := lockbox.New[string](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[string](dial...), lockbox.WithEndpoints[string](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}))

	require.NoError(t, e)
	defer p.Close(t.Context())

	require.NoError(t, p.Put(t.Context(), "key", "value"))
	v, e := p.Get(t.Context(), "key")
	require.NoError(t, e)
	require.Equal(t, "value", v.Value)
	v.Clear()
	require.NoError(t, p.Delete(t.Context(), "key"))
}
