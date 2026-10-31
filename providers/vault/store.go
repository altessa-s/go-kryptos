// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package vault

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
	"github.com/altessa-s/go-kryptos/internal/zero"

	kryptos "github.com/altessa-s/go-kryptos"
	api "github.com/hashicorp/vault/api"
)

// Storage adapts Vault KV v2. The injected client remains owned by the caller.
type Storage[T any] struct {
	client *api.Client
	kv     *api.KVv2
	o      *options[T]
	closed atomic.Bool
}

func New[T any](client *api.Client, opts ...Option[T]) (*Storage[T], error) {
	if client == nil {
		return nil, kryptos.ErrInvalidOptions
	}
	o := newOptions(opts...)
	if o.mountPath == "" || strings.Contains(o.mountPath, "..") || strings.Contains(o.secretPath, "..") || strings.HasPrefix(o.secretPath, "/") {
		return nil, kryptos.ErrInvalidOptions
	}
	if o.keyCodec == nil {
		o.keyCodec = base64key.New()
	}
	if o.valueCodec == nil {
		o.valueCodec = jsonvalue.New[T]()
	}
	return &Storage[T]{client: client, kv: client.KVv2(o.mountPath), o: o}, nil
}
func (*Storage[T]) Name() string { return "vault" }
func (s *Storage[T]) check(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.closed.Load() {
		return kryptos.ErrClosed
	}
	return nil
}
func (s *Storage[T]) key(ctx context.Context, key string) (string, error) {
	if e := s.check(ctx); e != nil {
		return "", e
	}
	if e := kryptos.ValidateKey(key); e != nil {
		return "", e
	}
	k, e := s.o.keyCodec.Encode(key)
	if e != nil {
		return "", kryptos.ErrKeyEncoding
	}
	if strings.ContainsAny(k, "/\\") || k == "." || k == ".." || k == "" {
		return "", kryptos.ErrKeyEncoding
	}
	return path.Join(s.o.secretPath, k), nil
}
func (s *Storage[T]) Get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	p, e := s.key(ctx, key)
	if e != nil {
		return nil, e
	}
	v, e := s.kv.Get(ctx, p)
	if e != nil {
		return nil, classify(e)
	}
	if v == nil || v.Data == nil {
		return nil, kryptos.ErrNotFound
	}
	text, ok := v.Data["data"].(string)
	if !ok || v.VersionMetadata == nil {
		return nil, kryptos.ErrMalformed
	}
	b, e := base64.StdEncoding.DecodeString(text)
	defer clear(b)
	if e != nil {
		return nil, kryptos.ErrMalformed
	}
	value, e := s.o.valueCodec.Decode(b)
	defer zero.Clear(&value)
	if e != nil {
		return nil, kryptos.ErrValueDecoding
	}
	out := kryptos.NewSecret(key, value, nil, strconv.Itoa(v.VersionMetadata.Version))
	out.EncodedKey = path.Base(p)
	return out, nil
}
func (s *Storage[T]) List(ctx context.Context) (out []*kryptos.Secret[T], err error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	defer func() {
		if err != nil {
			for _, s := range out {
				s.Clear()
			}
			out = nil
		}
	}()
	result, e := s.client.Logical().ListWithContext(ctx, path.Join(s.o.mountPath, "metadata", s.o.secretPath))
	if e != nil {
		return nil, classify(e)
	}
	if result == nil {
		return nil, nil
	}
	keys, ok := result.Data["keys"].([]any)
	if !ok {
		return nil, kryptos.ErrMalformed
	}
	for _, raw := range keys {
		k, ok := raw.(string)
		if !ok {
			return out, kryptos.ErrMalformed
		}
		key, e := s.o.keyCodec.Decode(k)
		if e != nil {
			return out, kryptos.ErrKeyDecoding
		}
		v, e := s.Get(ctx, key)
		if errors.Is(e, kryptos.ErrNotFound) {
			continue
		}
		if e != nil {
			return out, e
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Storage[T]) Put(ctx context.Context, key string, value T) error {
	_, err := s.PutResult(ctx, key, value)
	return err
}

// PutResult writes a value and reports backend existence before the write.
func (s *Storage[T]) PutResult(ctx context.Context, key string, value T) (result kryptos.WriteResult, err error) {
	p, e := s.key(ctx, key)
	if e != nil {
		return result, e
	}
	if e = kryptos.CheckPayload(value); e != nil {
		return result, e
	}
	owned := kryptos.NewSecret(key, value, nil, "")
	defer owned.Clear()
	b, e := s.o.valueCodec.Encode(owned.Value)
	defer clear(b)
	if e != nil {
		return result, kryptos.ErrValueEncoding
	}
	meta, e := s.kv.GetMetadata(ctx, p)
	if e != nil && !errors.Is(classify(e), kryptos.ErrNotFound) {
		return result, classify(e)
	}
	result.Created = meta == nil
	data := map[string]any{"data": map[string]any{"data": base64.StdEncoding.EncodeToString(b)}}
	if s.o.checkAndSet {
		version := 0
		if meta != nil {
			version = meta.CurrentVersion
		}
		data["options"] = map[string]any{"cas": version}
	}
	if e = s.write(ctx, p, data); e != nil {
		return result, e
	}
	if s.o.destroyPreviousVersions && meta != nil {
		var versions []int
		for _, v := range meta.Versions {
			if !v.Destroyed {
				versions = append(versions, v.Version)
			}
		}
		if len(versions) > 0 {
			if e = s.kv.Destroy(ctx, p, versions); e != nil {
				return result, &kryptos.CommittedError{Err: kryptos.ErrCleanupFailed}
			}
		}
	}
	return result, nil
}
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	p, e := s.key(ctx, key)
	if e != nil {
		return e
	}
	_, e = s.kv.GetMetadata(ctx, p)
	if e != nil {
		return classify(e)
	}
	return classify(s.kv.DeleteMetadata(ctx, p))
}
func (s *Storage[T]) Ping(ctx context.Context) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	_, e := s.client.Auth().Token().LookupSelfWithContext(ctx)
	return classify(e)
}
func (s *Storage[T]) Close(ctx context.Context) error {
	s.closed.Store(true)
	return nil
}
func classify(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) {
		return e
	}
	if errors.Is(e, api.ErrSecretNotFound) {
		return kryptos.ErrNotFound
	}
	if r, ok := errors.AsType[*api.ResponseError](e); ok {
		if r.StatusCode == http.StatusForbidden || r.StatusCode == http.StatusUnauthorized {
			return kryptos.ErrPermissionDenied
		}
		if r.StatusCode == http.StatusNotFound {
			return kryptos.ErrNotFound
		}
		if r.StatusCode == http.StatusBadRequest {
			for _, msg := range r.Errors {
				if strings.Contains(msg, "check-and-set") {
					return kryptos.ErrConflict
				}
			}
			return kryptos.ErrMalformed
		}
	}
	return kryptos.ErrUnavailable
}
