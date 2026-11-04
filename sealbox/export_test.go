// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

package sealbox

// NewWithKeyID is [New] with a custom key ID derivation, so tests can force a key ID collision.
var NewWithKeyID = newBox
