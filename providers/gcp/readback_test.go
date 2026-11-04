// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"

	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/manager"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

type panicDecoder struct{}

func (panicDecoder) Encode(v string) ([]byte, error) { return jsonvalue.New[string]().Encode(v) }
func (panicDecoder) Decode([]byte) (string, error)   { panic(kryptostest.Canary) }

func TestCommittedWriteDecoderPanic(t *testing.T) {
	t.Parallel()

	server := fake.New()
	conn, closeServer, err := server.Connect()
	require.NoError(t, err)

	defer closeServer()

	p, err := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithGRPCConn(conn)), gcp.WithValueCodec[string](panicDecoder{}))
	require.NoError(t, err)

	var logs bytes.Buffer

	m, err := manager.New(p, manager.WithLogger[string](slog.New(slog.NewTextHandler(&logs, nil))))

	require.NoError(t, err)
	defer m.Close(t.Context())

	sub, err := m.Watch(t.Context(), manager.WithSkipInitial())
	require.NoError(t, err)

	defer sub.Close()

	for _, want := range []kryptos.EventType{kryptos.Created, kryptos.Updated} {
		err = m.Put(t.Context(), "key", "value")
		require.ErrorIs(t, err, kryptos.ErrPanicked)
		_, committed := errors.AsType[*kryptos.CommittedError](err)
		require.True(t, committed)
		require.NotContains(t, err.Error(), kryptostest.Canary)
		got, err := m.Cached("key")
		require.NoError(t, err)
		require.Equal(t, "value", got.Value)
		got.Clear()

		select {
		case event := <-sub.Events():
			require.Equal(t, want, event.Type)
			require.Equal(t, "value", event.Secret.Value)
			event.Clear()
		default:
			t.Fatal("missing committed write event")
		}
	}

	require.NotContains(t, logs.String(), kryptostest.Canary)
}
