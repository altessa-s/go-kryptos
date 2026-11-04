// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package gcp

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
	"github.com/altessa-s/go-kryptos/internal/zero"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	pb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	kryptos "github.com/altessa-s/go-kryptos"
)

// Storage adapts Google Secret Manager; writes are unconditional version additions.
type Storage[T any] struct {
	client   *secretmanager.Client
	project  string
	o        *options[T]
	closed   atomic.Bool
	once     sync.Once
	closeErr error
}

func New[T any](ctx context.Context, projectID string, credentialsJSON []byte, opts ...Option[T]) (*Storage[T], error) {
	o := newOptions(opts...)
	if o.clearCredentials {
		defer clear(credentialsJSON)
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`).MatchString(projectID) {
		return nil, kryptos.ErrInvalidOptions
	}
	var credentials struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(credentialsJSON, &credentials) != nil || credentials.Type != "service_account" {
		return nil, kryptos.ErrInvalidOptions
	}
	if o.keyCodec == nil {
		o.keyCodec = base64key.New()
	}
	if o.valueCodec == nil {
		o.valueCodec = jsonvalue.New[T]()
	}
	o.labels = maps.Clone(o.labels)
	for k, v := range o.labels {
		if !label(k) || !label(v) {
			return nil, kryptos.ErrInvalidOptions
		}
	}
	//nolint:staticcheck // The credential type is explicitly restricted to service_account above.
	clientOpts := append([]option.ClientOption{option.WithCredentialsJSON(credentialsJSON)}, o.clientOptions...)
	client, e := secretmanager.NewClient(ctx, clientOpts...)
	if e != nil {
		return nil, kryptos.ErrUnavailable
	}
	return &Storage[T]{client: client, project: "projects/" + projectID, o: o}, nil
}
func (*Storage[T]) Name() string { return "gcp" }
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
	if e != nil || k == "" || strings.ContainsAny(k, "/\\") {
		return "", kryptos.ErrKeyEncoding
	}
	return s.project + "/secrets/" + k, nil
}
func (s *Storage[T]) Get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	name, e := s.key(ctx, key)
	if e != nil {
		return nil, e
	}
	v, e := s.client.AccessSecretVersion(ctx, &pb.AccessSecretVersionRequest{Name: name + "/versions/latest"})
	if e != nil {
		return nil, classify(e)
	}
	if v == nil || v.Payload == nil {
		return nil, kryptos.ErrMalformed
	}
	defer clear(v.Payload.Data)
	value, e := s.o.valueCodec.Decode(v.Payload.Data)
	defer zero.Clear(&value)
	if e != nil {
		return nil, kryptos.ErrValueDecoding
	}
	if e := kryptos.CheckPayload(value); e != nil {
		return nil, e
	}
	out := kryptos.NewSecret(key, value, nil, v.Name)
	out.EncodedKey = path.Base(name)
	return out, nil
}
func (s *Storage[T]) Put(ctx context.Context, key string, value T) error {
	_, err := s.PutResult(ctx, key, value)
	return err
}

// PutResult writes a value and reports backend existence before the write.
func (s *Storage[T]) PutResult(ctx context.Context, key string, value T) (result kryptos.WriteResult, err error) {
	name, e := s.key(ctx, key)
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
	_, e = s.client.GetSecret(ctx, &pb.GetSecretRequest{Name: name})
	result.Created = status.Code(e) == codes.NotFound
	if status.Code(e) == codes.NotFound {
		_,
			e = s.client.CreateSecret(ctx,
			&pb.CreateSecretRequest{Parent: s.project,
				SecretId: path.Base(name),
				Secret: &pb.Secret{Labels: s.o.labels,
					Replication: &pb.Replication{Replication: &pb.Replication_Automatic_{Automatic: &pb.Replication_Automatic{}}}}})
		if status.Code(e) == codes.AlreadyExists {
			result.Created = false
			e = nil
		}
	}
	if e != nil {
		return result, classify(e)
	}
	_, e = s.client.AddSecretVersion(ctx, &pb.AddSecretVersionRequest{Parent: name, Payload: &pb.SecretPayload{Data: b}})
	return result, classifyMutation(e)
}
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	name, e := s.key(ctx, key)
	if e != nil {
		return e
	}
	return classifyMutation(s.client.DeleteSecret(ctx, &pb.DeleteSecretRequest{Name: name}))
}
func (s *Storage[T]) List(ctx context.Context) (out []*kryptos.Secret[T], err error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	transferred := false
	defer func() {
		if !transferred {
			for _, v := range out {
				v.Clear()
			}
			out = nil
		}
	}()
	it := s.client.ListSecrets(ctx, &pb.ListSecretsRequest{Parent: s.project, Filter: buildFilter(s.o.labels)})
	for {
		secret, e := it.Next()
		if errors.Is(e, iterator.Done) {
			transferred = true
			return out, nil
		}
		if e != nil {
			return out, classify(e)
		}
		key, e := s.o.keyCodec.Decode(path.Base(secret.Name))
		if e != nil || kryptos.ValidateKey(key) != nil {
			if s.o.ignoreInvalidKeys {
				continue
			}
			return out, kryptos.ErrKeyDecoding
		}
		v, e := s.Get(ctx, key)
		if errors.Is(e, kryptos.ErrNotFound) || errors.Is(e, kryptos.ErrFailedPrecondition) {
			if s.o.metrics != nil {
				s.o.metrics.Skipped()
			}
			continue
		}
		if e != nil {
			return out, e
		}
		out = append(out, v)
	}
}
func (s *Storage[T]) Ping(ctx context.Context) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	_, e := s.client.ListSecrets(ctx, &pb.ListSecretsRequest{Parent: s.project, PageSize: 1}).Next()
	if errors.Is(e, iterator.Done) {
		return nil
	}
	return classify(e)
}
func (s *Storage[T]) Close(ctx context.Context) error {
	s.once.Do(func() { s.closed.Store(true); s.closeErr = s.client.Close() })
	return s.closeErr
}
func classify(e error) error {
	if e == nil {
		return nil
	}
	switch status.Code(e) {
	case codes.PermissionDenied, codes.Unauthenticated:
		return kryptos.ErrPermissionDenied
	case codes.InvalidArgument:
		return kryptos.ErrMalformed
	case codes.FailedPrecondition:
		return kryptos.ErrFailedPrecondition
	case codes.NotFound:
		return kryptos.ErrNotFound
	case codes.AlreadyExists, codes.Aborted:
		return kryptos.ErrConflict
	case codes.Canceled:
		return context.Canceled
	case codes.DeadlineExceeded:
		return context.DeadlineExceeded
	default:
		return kryptos.ErrUnavailable
	}
}
func label(v string) bool {
	const maxLabelLength = 63
	if len(v) > maxLabelLength {
		return false
	}
	for _, c := range v {
		allowed := c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_' || c == '-'
		if !allowed {
			return false
		}
	}
	return true
}
func buildFilter(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, "labels."+k+"="+labels[k])
	}
	return strings.Join(parts, " AND ")
}
