// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"

	"github.com/altessa-s/go-kryptos/providers/lockbox"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestTokenOptionsAndValidation(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	doer := httpx.DoerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("transport failed") })
	for _, opts := range [][]lockbox.TokenOption{{lockbox.WithIAMEndpoint(":bad")}, {lockbox.WithRefreshBefore(-time.Second)}} {
		_, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey, opts...)
		require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	}
	_, e := lockbox.NewServiceAccountTokens(nil, "key", "account", pemKey)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	_, e = lockbox.NewServiceAccountTokens(doer, "bad/id", "account", pemKey)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	_, e = lockbox.NewServiceAccountTokens(doer, "key", "account", nil)
	require.ErrorIs(t, e, kryptos.ErrInvalidOptions)
	source, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey, lockbox.WithIAMEndpoint("https://iam.example.test/tokens"), lockbox.WithClock(kryptos.RealClock{}), lockbox.WithRefreshBefore(0))
	require.NoError(t, e)
	_, e = source.Token(t.Context())
	require.ErrorIs(t, e, kryptos.ErrUnavailable)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, e = source.Token(ctx)
	require.ErrorIs(t, e, context.Canceled)
	require.NoError(t, source.Close(t.Context()))
}
func TestTokenLifecycleCancellation(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	synctest.Test(t, func(t *testing.T) {
		doer := httpx.DoerFunc(func(r *http.Request) (*http.Response, error) { <-r.Context().Done(); return nil, r.Context().Err() })
		source, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)
		require.NoError(t, e)
		done := make(chan error, 1)
		go func() { _, e := source.Token(t.Context()); done <- e }()
		synctest.Wait()
		require.NoError(t, source.Close(t.Context()))
		require.Error(t, <-done)
		require.NoError(t, source.Close(t.Context()))
	})
}
func FuzzIAMResponse(f *testing.F) {
	f.Add(`{}`)
	f.Add(`{"iamToken":"test","expiresAt":"2099-01-01T00:00:00Z"}`)
	f.Fuzz(func(t *testing.T, body string) {
		if len(body) > 70000 {
			return
		}
		pemKey := key(t)
		doer := httpx.DoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		source, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)
		require.NoError(t, e)
		defer source.Close(t.Context())
		token, e := source.Token(t.Context())
		if e == nil {
			require.NotEmpty(t, token)
		}
	})
}

func TestMalformedTransportResponse(t *testing.T) {
	t.Parallel()
	source, e := lockbox.NewServiceAccountTokens(httpx.DoerFunc(func(*http.Request) (*http.Response, error) { return nil, nil }), "key", "account", key(t))
	require.NoError(t, e)
	defer source.Close(t.Context())
	_, e = source.Token(t.Context())
	require.ErrorIs(t, e, kryptos.ErrMalformed)
}
