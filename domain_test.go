// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package kryptos_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"

	kryptos "github.com/altessa-s/go-kryptos"
)

func TestSecretOwnershipAndRedaction(t *testing.T) {
	t.Parallel()

	input := []byte(kryptostest.Canary)
	s := kryptos.NewSecret("key", input, input, "1")
	s.EncodedKey = "encoded"
	cloned := s.Clone()
	kryptostest.AssertRedacted(t, s)
	b, e := json.Marshal(s)
	require.NoError(t, e)
	require.NotContains(t, string(b), kryptostest.Canary)

	var log bytes.Buffer
	slog.New(slog.NewJSONHandler(&log, nil)).Info("secret", "value", s)
	require.NotContains(t, log.String(), kryptostest.Canary)
	s.Clear()
	s.Clear()
	require.Equal(t, []byte(kryptostest.Canary), input)
	require.Equal(t, input, cloned.Value)
	cloned.Clear()

	var nilSecret *kryptos.Secret[string]
	require.Nil(t, nilSecret.Clone())
	nilSecret.Clear()

	event := kryptos.Event[[]byte]{Secret: kryptos.NewSecret("key", input, nil, "")}
	event.Clear()
	event.Clear()
}
func TestValidateKey(t *testing.T) {
	t.Parallel()

	for _, k := range []string{"ab", "ns:key", "a_b.c-d"} {
		require.NoError(t, kryptos.ValidateKey(k))
	}

	for _, k := range []string{"", "a", "..", "/ab", "a b", "💡"} {
		require.ErrorIs(t, kryptos.ValidateKey(k), kryptos.ErrInvalidKey)
	}
}
func TestCloneability(t *testing.T) {
	t.Parallel()
	require.ErrorIs(t, kryptos.CheckPayload(make(chan int)), kryptos.ErrUncloneablePayload)
	require.NoError(t, kryptos.CheckPayload(map[string]any{"key": []byte("value")}))

	e := &kryptos.CommittedError{Err: kryptos.ErrCleanupFailed}
	require.ErrorIs(t, e, kryptos.ErrCleanupFailed)
	require.NotEmpty(t, e.Error())
}
func ExampleNewSecret() {
	s := kryptos.NewSecret("api-key", []byte("example"), nil, "1")
	defer s.Clear()

	fmt.Println(s) // Output: [REDACTED]
}
func FuzzValidateKey(f *testing.F) {
	f.Add("ns:key")
	f.Add("..")
	f.Fuzz(func(t *testing.T, key string) {
		e := kryptos.ValidateKey(key)
		if e == nil {
			require.GreaterOrEqual(t, len(key), 2)
			require.LessOrEqual(t, len(key), 255)
			require.NotContains(t, key, "/")
			require.NotEqual(t, "..", key)
		}
	})
}

func FuzzSecretFormat(f *testing.F) {
	f.Add("payload")
	f.Fuzz(func(t *testing.T, payload string) {
		s := kryptos.NewSecret("key", payload, []byte(payload), "1")
		defer s.Clear()

		for _, format := range []string{"%v", "%+v", "%#v", "%s", "%q", "%x"} {
			require.Equal(t, "[REDACTED]", fmt.Sprintf(format, s))
		}
	})
}

func TestClockWait(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		clock := kryptos.RealClock{}
		start := clock.Now()
		require.NoError(t, clock.Wait(t.Context(), time.Second))
		require.Equal(t, time.Second, clock.Now().Sub(start))
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		require.ErrorIs(t, clock.Wait(ctx, time.Hour), context.Canceled)
	})
}
