// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

//go:build integration

package gcp_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/altessa-s/go-kryptos/providers/gcp"
	"github.com/altessa-s/go-kryptos/providers/gcp/fake"

	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	pb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
)

func TestIntegrationTLS(t *testing.T) {
	t.Parallel()
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Skipf("TCP listener unavailable: %v", e)
	}
	defer listener.Close()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, e)
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"fake"}, NotBefore: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), NotAfter: time.Date(2100, 1, 1, 0, 0, 0, 0, time.UTC), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, e := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, e)
	cert, e := x509.ParseCertificate(der)
	require.NoError(t, e)
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})))
	pb.RegisterSecretManagerServiceServer(server, fake.New())
	go func() { _ = server.Serve(listener) }()
	defer server.Stop()
	p, e := gcp.New[string](t.Context(), "test-project", []byte(`{"type":"service_account"}`), gcp.WithClientOptions[string](option.WithEndpoint(listener.Addr().String()), option.WithoutAuthentication(), option.WithGRPCDialOption(grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, ServerName: "fake", RootCAs: roots})))))
	require.NoError(t, e)
	defer p.Close(t.Context())
	require.NoError(t, p.Put(t.Context(), "key", "value"))
	v, e := p.Get(t.Context(), "key")
	require.NoError(t, e)
	require.Equal(t, "value", v.Value)
	v.Clear()
	require.NoError(t, p.Delete(t.Context(), "key"))
}
