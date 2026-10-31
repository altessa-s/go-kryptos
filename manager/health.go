// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

import (
	"context"

	"github.com/altessa-s/go-kryptos"
)

// Health checks backend availability and cache freshness.
func (m *Manager[T]) Health(ctx context.Context) kryptos.HealthStatus {
	ctx, end, e := m.begin(ctx)
	if e != nil {
		return kryptos.NotServing
	}
	defer end()
	if m.p.Ping(ctx) != nil {
		return kryptos.NotServing
	}
	last := m.LastSync()
	if !last.IsZero() && m.o.clock.Now().Sub(last) > m.o.staleAfter {
		return kryptos.Degraded
	}
	return kryptos.Serving
}
