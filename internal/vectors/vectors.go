// Package vectors embeds the vendored conformance vectors (a copy of janzeer-sdk-conformance/vectors, checked by
// SHA256SUMS) for the tests of this module.
package vectors

import _ "embed"

// WalletParity is wallet-parity-vectors.json.
//
//go:embed wallet-parity-vectors.json
var WalletParity []byte

// VaultFixture is vault-fixture.json.
//
//go:embed vault-fixture.json
var VaultFixture []byte
