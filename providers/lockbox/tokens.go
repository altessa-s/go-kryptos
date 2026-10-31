// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/altessa-s/go-atlas-core/runtime/singleflight"

	"github.com/altessa-s/go-atlas-net/httpx"

	kryptos "github.com/altessa-s/go-kryptos"
)

// ServiceAccountTokens exchanges signed JWTs through an injected HTTP transport.
// A single lifecycle-bound refresh is shared by all callers; no per-RPC refresh goroutine is retained.
type ServiceAccountTokens struct {
	mu               sync.Mutex
	key              []byte
	keyID, accountID string
	doer             httpx.Doer
	o                *tokenOptions
	token            string
	expires          time.Time
	closed           bool
	group            *singleflight.Group[string, string]
	cancel           context.CancelFunc
	done             chan struct{}
	once             sync.Once
}

func NewServiceAccountTokens(doer httpx.Doer, keyID, accountID string, pemKey []byte, opts ...TokenOption) (*ServiceAccountTokens, error) {
	if doer == nil || !validID(keyID, maxAccountIDLength, true) || !validID(accountID, maxAccountIDLength, true) {
		return nil, kryptos.ErrInvalidOptions
	}
	if parsed, e := jwt.ParseRSAPrivateKeyFromPEM(pemKey); e != nil || parsed.N.BitLen() < minRSAKeyBits || parsed.Validate() != nil {
		return nil, kryptos.ErrInvalidOptions
	}
	o := newTokenOptions(opts...)
	u, e := url.Parse(o.iamEndpoint)
	if e != nil || u.Host == "" || u.Scheme != "https" || o.refreshBefore < 0 {
		return nil, kryptos.ErrInvalidOptions
	}
	if o.clock == nil {
		o.clock = kryptos.RealClock{}
	}
	life, cancel := context.WithCancel(context.Background())
	return &ServiceAccountTokens{key: slices.Clone(pemKey),
			keyID:     keyID,
			accountID: accountID,
			doer:      doer,
			o:         o,
			group: singleflight.New[string,
				string](singleflight.WithLifecycle(life)),
			cancel: cancel,
			done:   make(chan struct{})},
		nil
}
func (t *ServiceAccountTokens) Token(ctx context.Context) (string, error) {
	if e := ctx.Err(); e != nil {
		return "", e
	}
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return "", kryptos.ErrClosed
	}
	if t.token != "" && t.o.clock.Now().Add(t.o.refreshBefore).Before(t.expires) {
		v := t.token
		t.mu.Unlock()
		return v, nil
	}
	t.mu.Unlock()
	value, err := t.group.Do(ctx, "iam", func(work context.Context) (value string, err error) {
		defer func() {
			if recover() != nil {
				value, err = "", kryptos.ErrPanicked
			}
		}()
		return t.refresh(work)
	})
	if err != nil && ctx.Err() == nil {
		t.mu.Lock()
		defer t.mu.Unlock()
		if !t.closed && t.token != "" && t.o.clock.Now().Before(t.expires) {
			return t.token, nil
		}
	}
	return value, err
}
func (t *ServiceAccountTokens) refresh(ctx context.Context) (string, error) {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return "", kryptos.ErrClosed
	}
	key := slices.Clone(t.key)
	t.mu.Unlock()
	defer clear(key)
	parsed, e := jwt.ParseRSAPrivateKeyFromPEM(key)
	if e != nil {
		return "", kryptos.ErrInvalidOptions
	}
	now := t.o.clock.Now()
	signed := jwt.NewWithClaims(jwt.SigningMethodPS256,
		jwt.RegisteredClaims{Issuer: t.accountID,
			Audience:  jwt.ClaimStrings{DefaultIAMEndpoint},
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour))})
	signed.Header["kid"] = t.keyID
	token, e := signed.SignedString(parsed)
	if e != nil {
		return "", kryptos.ErrUnavailable
	}
	body, e := json.Marshal(struct {
		JWT string `json:"jwt"`
	}{token})
	if e != nil {
		return "", kryptos.ErrMalformed
	}
	defer clear(body)
	req, e := http.NewRequestWithContext(ctx, http.MethodPost, t.o.iamEndpoint, bytes.NewReader(body))
	if e != nil {
		return "", kryptos.ErrInvalidOptions
	}
	req.Header.Set("Content-Type", "application/json")
	resp, e := t.doer.Do(req)
	if e != nil {
		return "", kryptos.ErrUnavailable
	}
	if resp == nil || resp.Body == nil {
		return "", kryptos.ErrMalformed
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusUnauthorized {
		return "", kryptos.ErrPermissionDenied
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", kryptos.ErrUnavailable
	}
	const limit = 64 * 1024
	b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if e != nil {
		return "", kryptos.ErrUnavailable
	}
	defer clear(b)
	if len(b) > limit {
		return "", kryptos.ErrMalformed
	}
	var result struct {
		Token   string    `json:"iamToken"`
		Expires time.Time `json:"expiresAt"`
	}
	if json.Unmarshal(b, &result) != nil || result.Token == "" {
		return "", kryptos.ErrMalformed
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return "", kryptos.ErrClosed
	}
	if !result.Expires.After(t.o.clock.Now()) {
		return "", kryptos.ErrMalformed
	}
	t.token = result.Token
	t.expires = result.Expires
	return result.Token, nil
}

// Close cancels refresh, clears owned key bytes, and waits within the caller's deadline.
func (t *ServiceAccountTokens) Close(ctx context.Context) error {
	t.once.Do(func() {
		t.mu.Lock()
		t.closed = true
		clear(t.key)
		t.key = nil
		t.token = ""
		t.cancel()
		t.group.Close()
		t.mu.Unlock()
		go func() { t.group.Wait(); close(t.done) }()
	})
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.done:
		return nil
	}
}
