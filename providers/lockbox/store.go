// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package lockbox

import (
	"context"
	"errors"
	"maps"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/altessa-s/go-kryptos/codec/base64key"
	"github.com/altessa-s/go-kryptos/codec/jsonvalue"
	"github.com/altessa-s/go-kryptos/internal/zero"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"

	kryptos "github.com/altessa-s/go-kryptos"
	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	operation "github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
)

// TokenSource supplies IAM tokens without exposing credential storage.
// Its lifecycle belongs to the consumer, independently of a provider.
type TokenSource interface {
	Token(context.Context) (string, error)
}
type rpcCredentials struct{ tokens TokenSource }

func (c rpcCredentials) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	token, e := c.tokens.Token(ctx)
	if e != nil {
		return nil, e
	}
	return map[string]string{"authorization": "Bearer " + token}, nil
}
func (rpcCredentials) RequireTransportSecurity() bool { return true }

// Storage adapts the Lockbox secret, payload, and operation services.
type Storage[T any] struct {
	folder      string
	o           *options[T]
	ss          pb.SecretServiceClient
	ps          pb.PayloadServiceClient
	ops         operation.OperationServiceClient
	connections []*grpc.ClientConn
	closed      atomic.Bool
	once        sync.Once
	closeErr    error
	indexMu     sync.Mutex
	index       map[string]string
}

func New[T any](ctx context.Context, folderID string, tokens TokenSource, opts ...Option[T]) (*Storage[T], error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if !validID(folderID, maxFolderIDLength, false) || tokens == nil {
		return nil, kryptos.ErrInvalidOptions
	}
	o := newOptions(opts...)
	o.labels = maps.Clone(o.labels)
	if o.keyCodec == nil {
		o.keyCodec = base64key.New()
	}
	if o.valueCodec == nil {
		o.valueCodec = jsonvalue.New[T]()
	}
	if o.endpoints.Secret == "" {
		o.endpoints.Secret = "lockbox.api.cloud.yandex.net:443"
	}
	if o.endpoints.Payload == "" {
		o.endpoints.Payload = "payload.lockbox.api.cloud.yandex.net:443"
	}
	if o.endpoints.Operation == "" {
		o.endpoints.Operation = "operation.api.cloud.yandex.net:443"
	}
	dial := append([]grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(nil)),
		grpc.WithPerRPCCredentials(rpcCredentials{tokens: tokens})},
		o.dialOptions...)
	s := &Storage[T]{folder: folderID, o: o, index: make(map[string]string)}
	for _, endpoint := range []string{o.endpoints.Secret, o.endpoints.Payload, o.endpoints.Operation} {
		conn, e := grpc.NewClient(endpoint, dial...)
		if e != nil {
			for _, c := range s.connections {
				_ = c.Close()
			}
			return nil, kryptos.ErrUnavailable
		}
		s.connections = append(s.connections, conn)
	}
	s.ss = pb.NewSecretServiceClient(s.connections[0])
	s.ps = pb.NewPayloadServiceClient(s.connections[1])
	s.ops = operation.NewOperationServiceClient(s.connections[2])
	return s, nil
}
func (*Storage[T]) Name() string { return "lockbox" }
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
	return k, nil
}
func (s *Storage[T]) Get(ctx context.Context, key string) (*kryptos.Secret[T], error) {
	encoded, e := s.key(ctx, key)
	if e != nil {
		return nil, e
	}
	meta, e := s.find(ctx, encoded)
	if e != nil {
		return nil, e
	}
	return s.payload(ctx, key, meta)
}
func (s *Storage[T]) matches(meta *pb.Secret) bool {
	if meta.GetStatus() != pb.Secret_ACTIVE {
		return false
	}
	for k, want := range s.o.labels {
		if meta.GetLabels()[k] != want {
			return false
		}
	}
	return true
}
func (s *Storage[T]) payload(ctx context.Context, key string, meta *pb.Secret) (*kryptos.Secret[T], error) {
	if meta.GetCurrentVersion() == nil {
		return nil, kryptos.ErrNotFound
	}
	payload, e := s.ps.Get(ctx, &pb.GetPayloadRequest{SecretId: meta.Id, VersionId: meta.CurrentVersion.Id})
	if e != nil {
		return nil, classify(e)
	}
	for _, entry := range payload.GetEntries() {
		if entry.GetKey() != "Data" {
			continue
		}
		var b []byte
		switch value := entry.GetValue().(type) {
		case *pb.Payload_Entry_BinaryValue:
			b = value.BinaryValue
		case *pb.Payload_Entry_TextValue:
			b = []byte(value.TextValue)
		default:
			return nil, kryptos.ErrMalformed
		}
		defer clear(b)
		v, e := s.o.valueCodec.Decode(b)
		defer zero.Clear(&v)
		if e != nil {
			return nil, kryptos.ErrValueDecoding
		}
		out := kryptos.NewSecret(key, v, nil, payload.VersionId)
		out.EncodedKey = meta.Name
		return out, nil
	}
	return nil, kryptos.ErrMalformed
}
func (s *Storage[T]) Put(ctx context.Context, key string, value T) error {
	_, err := s.PutResult(ctx, key, value)
	return err
}

// PutResult writes a value and reports backend existence before the write.
func (s *Storage[T]) PutResult(ctx context.Context, key string, value T) (result kryptos.WriteResult, err error) {
	encoded, e := s.key(ctx, key)
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
	entries := []*pb.PayloadEntryChange{{Key: "Data", Value: &pb.PayloadEntryChange_BinaryValue{BinaryValue: b}}}
	meta, e := s.find(ctx, encoded)
	if e != nil && !errors.Is(e, kryptos.ErrNotFound) {
		return result, e
	}
	result.Created = meta == nil
	if meta == nil {
		op,
			createErr := s.ss.Create(ctx,
			&pb.CreateSecretRequest{FolderId: s.folder,
				Name:                  encoded,
				Labels:                s.o.labels,
				VersionPayloadEntries: entries,
				CreateVersion:         wrapperspb.Bool(true)})
		if createErr != nil && status.Code(createErr) != codes.AlreadyExists {
			return result, classifyMutation(createErr)
		}
		e = s.await(ctx, op, createErr)
		if e == nil {
			return result, nil
		}
		if status.Code(e) != codes.AlreadyExists {
			return result, classify(e)
		}
		result.Created = false
		meta, e = s.find(ctx, encoded)
		if e != nil {
			return result, e
		}
	}
	op, e := s.ss.AddVersion(ctx, &pb.AddVersionRequest{SecretId: meta.Id, PayloadEntries: entries})
	if e != nil {
		return result, classifyMutation(e)
	}
	return result, classify(s.await(ctx, op, nil))
}
func (s *Storage[T]) Delete(ctx context.Context, key string) error {
	encoded, e := s.key(ctx, key)
	if e != nil {
		return e
	}
	meta, e := s.find(ctx, encoded)
	if e != nil {
		return e
	}
	op, e := s.ss.Delete(ctx, &pb.DeleteSecretRequest{SecretId: meta.Id})
	err := classifyMutation(e)
	if e == nil {
		err = classify(s.await(ctx, op, nil))
	}
	if err == nil || errors.Is(err, kryptos.ErrNotFound) || errors.Is(err, kryptos.ErrIndeterminate) {
		s.indexMu.Lock()
		delete(s.index, encoded)
		s.indexMu.Unlock()
	}
	return err
}
func (s *Storage[T]) List(ctx context.Context) (out []*kryptos.Secret[T], err error) {
	if e := s.check(ctx); e != nil {
		return nil, e
	}
	defer func() {
		if err != nil {
			for _, v := range out {
				v.Clear()
			}
			out = nil
		}
	}()
	all, e := s.secrets(ctx)
	if e != nil {
		return nil, e
	}
	for _, meta := range all {
		if !s.matches(meta) {
			continue
		}
		key, e := s.o.keyCodec.Decode(meta.Name)
		if e != nil || kryptos.ValidateKey(key) != nil {
			if s.o.ignoreInvalidKeys {
				continue
			}
			return out, kryptos.ErrKeyDecoding
		}
		v, e := s.payload(ctx, key, meta)
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
func (s *Storage[T]) Ping(ctx context.Context) error {
	if e := s.check(ctx); e != nil {
		return e
	}
	_, e := s.ss.List(ctx, &pb.ListSecretsRequest{FolderId: s.folder, PageSize: 1})
	return classify(e)
}
func (s *Storage[T]) Close(ctx context.Context) error {
	s.once.Do(func() {
		s.closed.Store(true)
		for _, c := range s.connections {
			s.closeErr = errors.Join(s.closeErr, c.Close())
		}
	})
	return s.closeErr
}
func (s *Storage[T]) await(ctx context.Context, op *operation.Operation, err error) error {
	if err != nil {
		return err
	}
	if op == nil {
		return kryptos.ErrIndeterminate
	}
	id := op.Id
	delay := operationPollMin
	const maxPollFailures = 3
	failures := 0
	for !op.Done {
		if id == "" {
			return kryptos.ErrIndeterminate
		}
		if waitErr := (kryptos.RealClock{}).Wait(ctx, delay); waitErr != nil {
			return kryptos.ErrIndeterminate
		}
		delay = min(delay*operationPollFactor, time.Second)
		next, pollErr := s.ops.Get(ctx, &operation.GetOperationRequest{OperationId: id})
		if pollErr != nil {
			failures++
			if failures < maxPollFailures && (status.Code(pollErr) == codes.Unavailable || status.Code(pollErr) == codes.DeadlineExceeded) {
				continue
			}
			return kryptos.ErrIndeterminate
		}
		if next == nil {
			return kryptos.ErrIndeterminate
		}
		failures = 0
		op = next
	}
	if e := op.GetError(); e != nil {
		return status.ErrorProto(e)
	}
	return nil
}
func classify(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, kryptos.ErrMalformed) || errors.Is(e, kryptos.ErrIndeterminate) {
		return e
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

const (
	listPageSize        = 100
	operationPollMin    = 50 * time.Millisecond
	operationPollFactor = 2
)

const (
	maxFolderIDLength  = 50
	maxAccountIDLength = 100
	minRSAKeyBits      = 2048
)

func validID(v string, limit int, underscore bool) bool {
	if v == "" || len(v) > limit {
		return false
	}
	for _, c := range v {
		allowed := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || underscore && c == '_'
		if !allowed {
			return false
		}
	}
	return true
}
