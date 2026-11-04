// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

//go:build integration

package fake

import (
	"context"
	"net"

	"google.golang.org/grpc"
)

// ConnectTCP starts a TLS listener for tagged integration tests.
func (s *Server) ConnectTCP(ctx context.Context) ([]grpc.DialOption, func(), error) {
	var config net.ListenConfig

	listener, err := config.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}

	dial, stop := s.serve(listener, func(ctx context.Context, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "tcp", listener.Addr().String())
	})

	return dial, stop, nil
}
