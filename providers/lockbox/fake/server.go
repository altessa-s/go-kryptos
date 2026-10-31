// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fake

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"strconv"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/proto"

	pb "github.com/yandex-cloud/go-genproto/yandex/cloud/lockbox/v1"
	operation "github.com/yandex-cloud/go-genproto/yandex/cloud/operation"
)

// Server stores unique secret names and complete version histories.
type Server struct {
	mu                  sync.Mutex
	secrets             map[string]*entry
	sequence            int
	lists, payloadReads int
}
type entry struct {
	secret   *pb.Secret
	payloads []*pb.Payload
}
type secrets struct {
	pb.UnimplementedSecretServiceServer
	b *Server
}
type payloads struct {
	pb.UnimplementedPayloadServiceServer
	b *Server
}
type operations struct {
	operation.UnimplementedOperationServiceServer
}

func New() *Server { return &Server{secrets: map[string]*entry{}} }

type localToken struct{}

func (localToken) GetRequestMetadata(context.Context, ...string) (map[string]string, error) {
	return map[string]string{"authorization": "Bearer test"}, nil
}
func (localToken) RequireTransportSecurity() bool { return false }

// Connect serves all three services over one in-memory listener.
func (s *Server) Connect(opts ...grpc.ServerOption) ([]grpc.DialOption, func()) {
	listener := bufconn.Listen(bufferSize)
	return s.serve(listener, func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }, opts...)
}

func (s *Server) serve(listener net.Listener,
	dialer func(context.Context, string) (net.Conn, error), opts ...grpc.ServerOption) ([]grpc.DialOption, func()) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		panic(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1),
		DNSNames: []string{"fake"},
		NotBefore: time.Date(2000,
			1,
			1,
			0,
			0,
			0,
			0,
			time.UTC),
		NotAfter: time.Date(2100,
			1,
			1,
			0,
			0,
			0,
			0,
			time.UTC),
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	opts = append(opts, grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12,
		Certificates: []tls.Certificate{{Certificate: [][]byte{der},
			PrivateKey: key}}})))
	server := grpc.NewServer(opts...)
	pb.RegisterSecretServiceServer(server, &secrets{b: s})
	pb.RegisterPayloadServiceServer(server, &payloads{b: s})
	operation.RegisterOperationServiceServer(server, &operations{})
	go func() { _ = server.Serve(listener) }()
	return []grpc.DialOption{grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12,
			RootCAs:    roots,
			ServerName: "fake"})),
			grpc.WithPerRPCCredentials(localToken{}),
			grpc.WithContextDialer(dialer)},
		func() { server.Stop(); _ = listener.Close() }
}
func (s *secrets) List(context.Context, *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	s.b.lists++
	r := &pb.ListSecretsResponse{}
	for _, e := range s.b.secrets {
		r.Secrets = append(r.Secrets, proto.CloneOf(e.secret))
	}
	return r, nil
}
func (s *secrets) Get(_ context.Context, r *pb.GetSecretRequest) (*pb.Secret, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	for _, e := range s.b.secrets {
		if e.secret.Id == r.SecretId {
			return proto.CloneOf(e.secret), nil
		}
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (s *secrets) Create(_ context.Context, r *pb.CreateSecretRequest) (*operation.Operation, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	if s.b.secrets[r.Name] != nil {
		return nil, status.Error(codes.AlreadyExists, "name exists")
	}
	s.b.sequence++
	e := &entry{secret: &pb.Secret{Id: strconv.Itoa(s.b.sequence), Name: r.Name, Status: pb.Secret_ACTIVE, Labels: r.Labels}}
	add(e, r.VersionPayloadEntries)
	s.b.secrets[r.Name] = e
	return &operation.Operation{Id: "create", Done: true}, nil
}
func add(e *entry, changes []*pb.PayloadEntryChange) {
	id := strconv.Itoa(len(e.payloads) + 1)
	p := &pb.Payload{VersionId: id}
	for _, c := range changes {
		p.Entries = append(p.Entries, &pb.Payload_Entry{Key: c.Key, Value: &pb.Payload_Entry_TextValue{TextValue: c.GetTextValue()}})
	}
	e.payloads = append(e.payloads, p)
	e.secret.CurrentVersion = &pb.Version{Id: id}
}
func (s *secrets) AddVersion(_ context.Context, r *pb.AddVersionRequest) (*operation.Operation, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	for _, e := range s.b.secrets {
		if e.secret.Id == r.SecretId {
			add(e, r.PayloadEntries)
			return &operation.Operation{Id: "add", Done: true}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (s *secrets) Delete(_ context.Context, r *pb.DeleteSecretRequest) (*operation.Operation, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	for name, e := range s.b.secrets {
		if e.secret.Id == r.SecretId {
			delete(s.b.secrets, name)
			return &operation.Operation{Id: "delete", Done: true}, nil
		}
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (s *payloads) Get(_ context.Context, r *pb.GetPayloadRequest) (*pb.Payload, error) {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	s.b.payloadReads++
	for _, e := range s.b.secrets {
		if e.secret.Id == r.SecretId {
			for _, p := range e.payloads {
				if p.VersionId == r.VersionId {
					return proto.CloneOf(p), nil
				}
			}
		}
	}
	return nil, status.Error(codes.NotFound, "missing")
}
func (*operations) Get(context.Context, *operation.GetOperationRequest) (*operation.Operation, error) {
	return &operation.Operation{Done: true}, nil
}

const bufferSize = 1024 * 1024

// Versions returns the number of durable versions for an encoded secret name.
func (s *Server) Versions(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.secrets[name]; e != nil {
		return len(e.payloads)
	}
	return 0
}

// Counts returns listing and payload RPC counts.
func (s *Server) Counts() (int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lists, s.payloadReads
}

// Deactivate changes the status of a secret to exercise write policy.
func (s *Server) Deactivate(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[name].secret.Status = pb.Secret_INACTIVE
}
