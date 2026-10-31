// Copyright 2021-2026 ALTESSA SOLUTIONS INC.
// Use of this source code is governed by the MIT license that can be found in
// the LICENSE file.

package manager

//go:generate optgen generate --type=watchOptions --option-type=WatchOption --output=watch_options_gen.go
import kryptos "github.com/altessa-s/go-kryptos"

// OverflowPolicy selects which pending event is discarded when a buffer is full.
type OverflowPolicy int

const (
	DropNewest OverflowPolicy = iota
	DropOldest
)

type watchOptions struct {
	watchKeys   []string
	watchTypes  []kryptos.EventType
	buffer      int `optgen:"default=16"`
	overflow    OverflowPolicy
	skipInitial bool
}
