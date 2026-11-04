// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp_test

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"google.golang.org/api/option"

	"github.com/altessa-s/go-kryptos/codec"
	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	kryptos "github.com/altessa-s/go-kryptos"
)

func reviewProvider[T any](t *testing.T, codec codec.ValueCodec[T]) kryptos.Provider[T] {
	t.Helper()

	conn, stop, err := fake.New().Connect()
	require.NoError(t, err)
	t.Cleanup(stop)
	p, err := gcp.New[T](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[T](option.WithGRPCConn(conn)), gcp.WithValueCodec[T](codec))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, p.Close(t.Context())) })

	return p
}

type privatePayload struct{ data []byte }

func (p *privatePayload) Zeroize() { clear(p.data) }

type privateCodec struct{ decoded []byte }

func (*privateCodec) Encode(any) ([]byte, error) { return []byte("secret"), nil }
func (c *privateCodec) Decode([]byte) (any, error) {
	c.decoded = []byte("secret")
	return &privatePayload{data: c.decoded}, nil
}
func TestDecodedPayloadMustBeCloneable(t *testing.T) {
	t.Parallel()

	codec := &privateCodec{}
	p := reviewProvider[any](t, codec)
	require.NoError(t, p.Put(t.Context(), "key", "seed"))
	got, err := p.Get(t.Context(), "key")
	require.Nil(t, got)
	require.ErrorIs(t, err, kryptos.ErrUncloneablePayload)
	require.Equal(t, make([]byte, len(codec.decoded)), codec.decoded)
}

type listedPayload struct {
	data   []byte
	copies *[][]byte
}

func (p listedPayload) Clone() listedPayload {
	p.data = slices.Clone(p.data)
	*p.copies = append(*p.copies, p.data)

	return p
}
func (p *listedPayload) Zeroize() { clear(p.data) }

type listingCodec struct {
	calls  int
	fail   bool
	copies [][]byte
}

func (*listingCodec) Encode(listedPayload) ([]byte, error) { return []byte("secret"), nil }
func (c *listingCodec) Decode([]byte) (listedPayload, error) {
	c.calls++
	if c.calls == 2 {
		if c.fail {
			return listedPayload{}, kryptos.ErrMalformed
		}

		panic("decode failed")
	}

	return listedPayload{data: []byte("secret"), copies: &c.copies}, nil
}
func TestListClearsUndeliveredCopies(t *testing.T) {
	t.Parallel()

	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "panic", true: "error"}[fail], func(t *testing.T) {
			t.Parallel()

			codec := &listingCodec{fail: fail}
			p := reviewProvider[listedPayload](t, codec)
			require.NoError(t, p.Put(t.Context(), "first", listedPayload{copies: &codec.copies}))
			require.NoError(t, p.Put(t.Context(), "second", listedPayload{copies: &codec.copies}))
			codec.copies = nil

			if fail {
				got, err := p.List(t.Context())
				require.Nil(t, got)
				require.ErrorIs(t, err, kryptos.ErrValueDecoding)
			} else {
				require.Panics(t, func() { _, _ = p.List(t.Context()) })
			}

			require.Len(t, codec.copies, 1)
			require.Equal(t, []byte{0, 0, 0, 0, 0, 0}, codec.copies[0])
		})
	}
}
