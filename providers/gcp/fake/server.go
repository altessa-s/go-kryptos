// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package fake

import (
	"context"
	"net"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/types/known/emptypb"

	pb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

// Server implements a name-unique in-memory Secret Manager service for contracts.
type Server struct {
	pb.UnimplementedSecretManagerServiceServer
	mu      sync.Mutex
	secrets map[string]*entry
}
type entry struct {
	versions [][]byte
	labels   map[string]string
}

// New returns an empty server.
func New() *Server { return &Server{secrets: map[string]*entry{}} }

// Connect starts a socket-free gRPC transport over this backend.
func (s *Server) Connect(opts ...grpc.ServerOption) (*grpc.ClientConn, func(), error) {
	listener := bufconn.Listen(bufferSize)
	server := grpc.NewServer(opts...)

	pb.RegisterSecretManagerServiceServer(server, s)
	go func() { _ = server.Serve(listener) }()

	conn,
		e := grpc.NewClient("passthrough:///fake",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context,
			_ string) (net.Conn,
			error) {
			return listener.DialContext(ctx)
		}))

	return conn, func() {
		if conn != nil {
			_ = conn.Close()
		}

		server.Stop()

		_ = listener.Close()
	}, e
}

// GetSecret implements the Secret Manager GetSecret RPC.
func (s *Server) GetSecret(_ context.Context, r *pb.GetSecretRequest) (*pb.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := s.secrets[r.Name]
	if v == nil {
		return nil, status.Error(codes.NotFound, "missing")
	}

	return &pb.Secret{Name: r.Name, Labels: v.labels}, nil
}

// CreateSecret implements the Secret Manager CreateSecret RPC.
func (s *Server) CreateSecret(_ context.Context, r *pb.CreateSecretRequest) (*pb.Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name := r.Parent + "/secrets/" + r.SecretId
	if s.secrets[name] != nil {
		return nil, status.Error(codes.AlreadyExists, "exists")
	}

	s.secrets[name] = &entry{labels: r.Secret.GetLabels()}

	return &pb.Secret{Name: name}, nil
}

// AddSecretVersion implements the Secret Manager AddSecretVersion RPC.
func (s *Server) AddSecretVersion(_ context.Context, r *pb.AddSecretVersionRequest) (*pb.SecretVersion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := s.secrets[r.Parent]
	if v == nil {
		return nil, status.Error(codes.NotFound, "missing")
	}

	v.versions = append(v.versions, slices.Clone(r.Payload.Data))

	return &pb.SecretVersion{Name: r.Parent + "/versions/" + strconv.Itoa(len(v.versions))}, nil
}

// AccessSecretVersion implements the Secret Manager AccessSecretVersion RPC.
func (s *Server) AccessSecretVersion(_ context.Context, r *pb.AccessSecretVersionRequest) (*pb.AccessSecretVersionResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	name, _, _ := strings.Cut(r.Name, "/versions/")

	v := s.secrets[name]
	if v == nil || len(v.versions) == 0 {
		return nil, status.Error(codes.NotFound, "missing")
	}

	return &pb.AccessSecretVersionResponse{Name: name + "/versions/" + strconv.Itoa(len(v.versions)),
			Payload: &pb.SecretPayload{Data: slices.Clone(v.versions[len(v.versions)-1])}},
		nil
}

// DeleteSecret implements the Secret Manager DeleteSecret RPC.
func (s *Server) DeleteSecret(_ context.Context, r *pb.DeleteSecretRequest) (*emptypb.Empty, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	v := s.secrets[r.Name]
	if v == nil {
		return nil, status.Error(codes.NotFound, "missing")
	}

	for _, b := range v.versions {
		clear(b)
	}

	delete(s.secrets, r.Name)

	return &emptypb.Empty{}, nil
}

// ListSecrets implements the Secret Manager ListSecrets RPC.
func (s *Server) ListSecrets(_ context.Context, _ *pb.ListSecretsRequest) (*pb.ListSecretsResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	names := make([]string, 0, len(s.secrets))
	for name := range s.secrets {
		names = append(names, name)
	}

	sort.Strings(names)

	out := &pb.ListSecretsResponse{}
	for _, name := range names {
		out.Secrets = append(out.Secrets, &pb.Secret{Name: name})
	}

	return out, nil
}

// Versions returns the number of persisted versions of a name.
func (s *Server) Versions(name string) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if v := s.secrets[name]; v != nil {
		return len(v.versions)
	}

	return 0
}

const bufferSize = 1024 * 1024
