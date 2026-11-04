// Copyright 2021-2026 ALTESSA SOLUTIONS INC. All rights reserved.
// Use of this source code is governed by license that can be found in
// the LICENSE file.

// Package sealbox seals and opens small secrets at rest (API keys, webhook secrets, client credentials) with AES-256-GCM and a
// keyring of master keys.
//
// # Keys and rotation
//
// [New] takes the current master key and any number of previous ones, each the standard base64 encoding of 32 random bytes (for
// example from configuration or a secret manager). The current key seals; every configured key opens. To rotate, configure a new
// current key and move the old one to the previous keys; [Box.Current] tells which stored values still use an older key so they can be
// re-sealed on their next save, after which the old key can be dropped. A key given more than once is kept once; two different keys
// whose [KeyID] collide are rejected with [ErrKeyIDCollision], since a sealed value names its key only by that ID. An empty current
// key yields a nil [*Box]: [Box.Seal] and [Box.Open] fail with [ErrNoKey] and [Box.Current] and [Box.Opens] report false, so a service
// can run without secrets storage configured.
//
// # Nonce budget
//
// Every seal draws a random 96-bit GCM nonce. NIST SP 800-38D caps random-nonce use at 2^32 seals per key: past that the chance of a
// nonce repeating, which breaks GCM's confidentiality and authenticity for that key, exceeds 2^-32. The box does not count seals (a
// key is shared by every process and outlives each one), so the budget is the operator's: rotate the current key well before 2^32
// seals have been made with it, counted across all processes and all time. Sealing small secrets on save stays far below it; a
// workload that seals per request or per message must rotate on a schedule derived from its seal rate.
//
// # Associated data
//
// Every value is bound to associated data chosen by the caller: it opens only with the same bytes, so a ciphertext copied onto another
// record does not decrypt. Bind it to both the record identity and the field (for example "provider:<id>:apiKey"), so a value cannot be
// moved between records or between two sealed fields of one record.
//
// # Format
//
// A [Sealed] value holds three fields that are stored as they are:
//
//   - KeyID: the first 16 hex digits of the SHA-256 of the raw key ([KeyID]);
//   - Nonce: 12 random bytes, fresh for every seal;
//   - Ciphertext: the GCM output, the encrypted plaintext followed by the 16-byte tag.
//
// The format is stable: values sealed by earlier versions keep opening.
//
// # Usage
//
//	box, err := sealbox.New(cfg.MasterKey, cfg.PreviousMasterKeys...)
//	if err != nil {
//	    return err
//	}
//	sealed, err := box.Seal([]byte(apiKey), []byte("provider:"+id))
//	// store sealed.KeyID, sealed.Nonce, sealed.Ciphertext
//	plain, err := box.Open(sealed, []byte("provider:"+id))
package sealbox
