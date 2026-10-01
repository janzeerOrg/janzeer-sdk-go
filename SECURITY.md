# Security

## Reporting a vulnerability

Email **janzeeer@proton.me** with a description and, if possible, a minimal reproduction. Please do not open a public
issue for anything that could affect users' funds. We acknowledge reports within 3 working days.

## What this SDK does and does not do

- The SDK **never transmits private keys or mnemonics**. It derives keys and signs in your process; the node only
  ever receives a signed transaction and read queries. There is no server-side signing endpoint.
- `Account.PrivateKeyHex` exists for backup and export flows. Do not log it; printing an `Account` (`%v`, `%+v`,
  `%#v`) shows the address only. Prefer the `vault` package to store a mnemonic at rest.
- Signatures are deterministic (RFC 6979): the same transaction always produces the same signature, so a weak random
  number generator cannot leak the key through signing. Key *generation* uses `crypto/rand`.
- Cryptography comes from `github.com/decred/dcrd/dcrec/secp256k1` (pure Go, constant-time field arithmetic),
  `golang.org/x/crypto` and the Go standard library. The SDK adds no primitive of its own, and uses no cgo.
- Amounts are never floats: no function takes or returns a `float64`.
- The SDK trusts the node it talks to for chain data. Use HTTPS/WSS to a node you operate or trust.
