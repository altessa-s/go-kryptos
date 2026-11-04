// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-atlas-net/httpx"

	"github.com/altessa-s/go-kryptos/providers/lockbox"

	kryptos "github.com/altessa-s/go-kryptos"
)

func key(t *testing.T) []byte {
	t.Helper()

	k, e := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, e)

	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(k)})
}
func TestIAMStatusBoundsAndOwnership(t *testing.T) {
	t.Parallel()

	pemKey := key(t)
	for _, tc := range []struct {
		name, body string
		code       int
	}{{"denied", `{}`, 403}, {"empty", `{}`, 200}, {"oversized", strings.Repeat("x", 65537), 200}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			doer := httpx.DoerFunc(func(r *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: tc.code, Body: io.NopCloser(strings.NewReader(tc.body)), Header: http.Header{}}, nil
			})
			source, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)
			require.NoError(t, e)
			_, e = source.Token(t.Context())
			require.Error(t, e)
			require.NoError(t, source.Close(t.Context()))
			require.Contains(t, string(pemKey), "PRIVATE KEY")
			_, e = source.Token(t.Context())
			require.ErrorIs(t, e, kryptos.ErrClosed)
		})
	}
}
func TestIAMRefresh(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		doer := httpx.DoerFunc(func(r *http.Request) (*http.Response, error) {
			calls.Add(1)
			body := fmt.Sprintf(`{"iamToken":"test","expiresAt":%q}`, time.Now().Add(2*time.Minute).Format(time.RFC3339))

			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}}, nil
		})
		source, e := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)

		require.NoError(t, e)
		defer source.Close(context.WithoutCancel(t.Context()))

		for range 2 {
			token, e := source.Token(t.Context())
			require.NoError(t, e)
			require.Equal(t, "test", token)
		}

		require.EqualValues(t, 1, calls.Load())
		time.Sleep(90 * time.Second)

		_, e = source.Token(t.Context())
		require.NoError(t, e)
		require.EqualValues(t, 2, calls.Load())
	})
}

func TestRefreshFailureKeepsUnexpiredToken(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		doer := httpx.DoerFunc(func(*http.Request) (*http.Response, error) {
			if calls.Add(1) > 1 {
				return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			}

			body := fmt.Sprintf(`{"iamToken":"cached","expiresAt":%q}`, time.Now().Add(2*time.Minute).Format(time.RFC3339))

			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		source, err := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)

		require.NoError(t, err)
		defer source.Close(t.Context())

		token, err := source.Token(t.Context())
		require.NoError(t, err)
		require.Equal(t, "cached", token)
		time.Sleep(90 * time.Second)

		token, err = source.Token(t.Context())
		require.NoError(t, err)
		require.Equal(t, "cached", token)
		time.Sleep(31 * time.Second)

		_, err = source.Token(t.Context())
		require.ErrorIs(t, err, kryptos.ErrUnavailable)
	})
}

func TestIAMConcurrentPanicSanitized(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})

		var calls atomic.Int32

		doer := httpx.DoerFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			<-release
			panic("credential-canary")
		})
		source, err := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)

		require.NoError(t, err)
		defer source.Close(t.Context())

		results := make(chan error, 12)
		for range cap(results) {
			go func() { _, err := source.Token(t.Context()); results <- err }()
		}

		synctest.Wait()
		close(release)

		for range cap(results) {
			err := <-results
			require.ErrorIs(t, err, kryptos.ErrPanicked)
			require.NotContains(t, err.Error(), "credential-canary")
		}

		require.EqualValues(t, 1, calls.Load())
	})
}

func TestIAMRejectsExpiredDelayedResponse(t *testing.T) {
	t.Parallel()
	pemKey := key(t)
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32

		doer := httpx.DoerFunc(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			body := fmt.Sprintf(`{"iamToken":"expired","expiresAt":%q}`, time.Now().Add(time.Second).Format(time.RFC3339))
			time.Sleep(2 * time.Second)

			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})
		source, err := lockbox.NewServiceAccountTokens(doer, "key", "account", pemKey)

		require.NoError(t, err)
		defer source.Close(t.Context())

		for range 2 {
			value, err := source.Token(t.Context())
			require.ErrorIs(t, err, kryptos.ErrMalformed)
			require.Empty(t, value)
		}

		require.EqualValues(t, 2, calls.Load())
	})
}
