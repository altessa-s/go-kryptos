// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/kryptostest"
	"github.com/altessa-s/go-kryptos/providers/lockbox"
	"github.com/altessa-s/go-kryptos/providers/lockbox/fake"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	kryptos "github.com/altessa-s/go-kryptos"
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
			dial, close := fake.New().Connect(grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
				if fail && strings.HasSuffix(info.FullMethod, "/Create") {
					return nil, status.Error(codes.PermissionDenied, "denied")
				}
				return next(ctx, req)
			}))
			defer close()
			p, err := lockbox.New[[]byte](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[[]byte](dial...), lockbox.WithEndpoints[[]byte](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithValueCodec[[]byte](codec))
			require.NoError(t, err)
			defer p.Close(t.Context())
			input := []byte{0xff, 0x00, 0xfe, 0x80}
			err = p.Put(t.Context(), "key", input)
			if fail {
				require.ErrorIs(t, err, kryptos.ErrPermissionDenied)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, []byte{0xff, 0x00, 0xfe, 0x80}, input)
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

type rejectedCodec struct{ decoded []byte }

func (*rejectedCodec) Encode(v []byte) ([]byte, error) { return slices.Clone(v), nil }
func (c *rejectedCodec) Decode(v []byte) ([]byte, error) {
	c.decoded = slices.Clone(v)
	return c.decoded, kryptos.ErrMalformed
}
func TestRejectedDecodedValueCleared(t *testing.T) {
	t.Parallel()
	codec := &rejectedCodec{}
	dial, stop := fake.New().Connect()
	t.Cleanup(stop)
	p, err := lockbox.New[[]byte](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[[]byte](dial...), lockbox.WithEndpoints[[]byte](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithValueCodec[[]byte](codec))
	require.NoError(t, err)
	defer p.Close(t.Context())
	require.NoError(t, p.Put(t.Context(), "key", []byte{0xff, 0x80}))
	_, err = p.Get(t.Context(), "key")
	require.ErrorIs(t, err, kryptos.ErrValueDecoding)
	require.Equal(t, []byte{0, 0}, codec.decoded)
}

func TestBinaryIdentityContract(t *testing.T) {
	t.Parallel()
	kryptostest.RunProviderContract(t, func(t testing.TB) kryptos.Provider[[]byte] {
		t.Helper()
		codec := &identityCodec{}
		dial, stop := fake.New().Connect()
		t.Cleanup(stop)
		p, err := lockbox.New[[]byte](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[[]byte](dial...), lockbox.WithEndpoints[[]byte](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithValueCodec[[]byte](codec))
		require.NoError(t, err)
		return p
	}, kryptostest.Caps{}, []byte{0xff, 0, 0x80, 0xfe})
}

type decodedPayload struct{ data []byte }

func (p *decodedPayload) Zeroize()             { clear(p.data) }
func (*decodedPayload) Clone() *decodedPayload { panic("clone failed") }

type payloadDecoder struct {
	decoded *decodedPayload
	fail    bool
}

func (*payloadDecoder) Encode(*decodedPayload) ([]byte, error) { return nil, kryptos.ErrValueEncoding }
func (c *payloadDecoder) Decode(v []byte) (*decodedPayload, error) {
	c.decoded = &decodedPayload{data: slices.Clone(v)}
	if c.fail {
		return c.decoded, kryptos.ErrMalformed
	}
	return c.decoded, nil
}
func TestDecodedZeroizerClearedOnFailure(t *testing.T) {
	t.Parallel()
	for _, fail := range []bool{true, false} {
		t.Run(map[bool]string{true: "decode-error", false: "clone-panic"}[fail], func(t *testing.T) {
			t.Parallel()
			dial, stop := fake.New().Connect()
			t.Cleanup(stop)
			codec := &identityCodec{}
			writer, err := lockbox.New[[]byte](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[[]byte](dial...), lockbox.WithEndpoints[[]byte](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithValueCodec[[]byte](codec))
			require.NoError(t, err)
			defer writer.Close(t.Context())
			require.NoError(t, writer.Put(t.Context(), "key", []byte{0xff, 0x80}))
			decoder := &payloadDecoder{fail: fail}
			reader, err := lockbox.New[*decodedPayload](t.Context(), "folder", tokens{}, lockbox.WithDialOptions[*decodedPayload](dial...), lockbox.WithEndpoints[*decodedPayload](lockbox.Endpoints{Secret: "passthrough:///secret", Payload: "passthrough:///payload", Operation: "passthrough:///operation"}), lockbox.WithValueCodec[*decodedPayload](decoder))
			require.NoError(t, err)
			defer reader.Close(t.Context())
			if fail {
				_, err = reader.Get(t.Context(), "key")
				require.ErrorIs(t, err, kryptos.ErrValueDecoding)
			} else {
				require.Panics(t, func() { _, _ = reader.Get(t.Context(), "key") })
			}
			require.Equal(t, []byte{0, 0}, decoder.decoded.data)
		})
	}
}
