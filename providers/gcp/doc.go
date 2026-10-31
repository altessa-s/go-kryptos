// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

// Package gcp adapts gcp secrets to the kryptos provider port.
//
// # Ownership
//
// Secret payloads must not be logged. Clear caller-owned results after use.
// See README.md for construction and lifecycle examples.
package gcp
