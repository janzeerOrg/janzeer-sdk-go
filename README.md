# janzeer-sdk-go

Official Go SDK for the [Janzeer](https://janzeer.org) blockchain — a stake-free, permissionless Layer-1 with
instant finality. Wallet keys and signing, transaction builders, a REST client, JSON-RPC over HTTP and WebSocket
subscriptions. Non-custodial by construction: keys never leave your process, the node only verifies.

![license](https://img.shields.io/badge/license-Apache--2.0-blue)
[![Go Reference](https://pkg.go.dev/badge/github.com/janzeerorg/janzeer-sdk-go.svg)](https://pkg.go.dev/github.com/janzeerorg/janzeer-sdk-go)

```bash
go get github.com/janzeerorg/janzeer-sdk-go@master
```

> **No tagged release yet.** `v0.1.0` follows shortly after the mainnet launch; until then `@master` pins the
> current commit.

## 60-second quickstart

```go
package main

import (
	"context"
	"fmt"
	"log"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/client"
	"github.com/janzeerorg/janzeer-sdk-go/rpc"
)

func main() {
	ctx := context.Background()
	c := client.New("https://onion.janzeer.org") // REST  (/api/v1)
	r := rpc.New("https://onion.janzeer.org")    // JSON-RPC (/rpc)
	me, err := janzeer.AccountFromMnemonic("abandon abandon … about", "")
	if err != nil {
		log.Fatal(err)
	}

	nonce, _ := c.Nonce(ctx, me.Address())
	tx, err := janzeer.NewTransfer(janzeer.Transfer{
		Common: janzeer.Common{From: me.Address(), Nonce: nonce},
		To:     "0x598b1301acef3baba6ce25e38dd17b723f7b98b1", Amount: "1.25", Data: "hello",
	})
	if err != nil {
		log.Fatal(err)
	}
	signed, _ := me.SignTx(tx)
	if _, err := c.Submit(ctx, signed); err != nil { // HTTP 201, or a typed error
		log.Fatal(err)
	}
	final, err := rpc.WaitForFinality(ctx, r, signed.Hash) // one or two 15-second slots
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("final in block", *final.BlockHeight)
}
```

That is the whole integration: derive → nonce → build → sign → submit → wait. A committed block is final (2f+1 BFT
commit), so there is no confirmation count.

## What is in the box

| Package | Contents | Guide |
|---|---|---|
| `janzeer` | `Account`, mnemonics, addresses, amounts, the `New…` transaction builders, `UnsignedTx` / `SignedTx`, error types, result models | [wallet-and-keys](docs/wallet-and-keys.md), [sending-transactions](docs/sending-transactions.md), [errors](docs/errors.md) |
| `janzeer/client` | REST: balances, nonces, transactions, validators, `Get` / `Post` for the rest | [reading-chain-data](docs/reading-chain-data.md) |
| `janzeer/rpc` | JSON-RPC over HTTP (`Client`, batches) and WebSocket (`WS`, `newBlocks` / `addressActivity`), `WaitForFinality` | [json-rpc-and-subscriptions](docs/json-rpc-and-subscriptions.md) |
| `janzeer/vault` | `Encrypt` / `Decrypt` (PBKDF2 + AES-GCM, compatible with the other SDKs) | [vault](docs/vault.md) |

## Money is never a float

Amounts are decimal **strings** (`"1.25"`). No function of this module takes or returns a `float64`: `0.1` is not
representable in binary, and a payment must not depend on how a float prints. Responses decode coin amounts into
`janzeer.Amount`, which keeps the exact digits the node wrote. `ToScaled` gives base units as a `*big.Int`.

## Design

- Every network call takes a `context.Context`. Clients have no global state and are safe for concurrent use.
- Errors are typed values: `errors.As(err, &nonce)` for `*janzeer.NonceMismatchError`, `*janzeer.TxRejectedError`,
  `*janzeer.APIError`, `*janzeer.RPCError`, `*janzeer.NetworkError`… — see [errors](docs/errors.md).
- No cgo: `CGO_ENABLED=0 go build` works, so it cross-compiles to every platform Go supports.
- Dependencies: `decred/dcrd/dcrec/secp256k1` (pure Go, RFC-6979), `golang.org/x/crypto` (keccak-256, PBKDF2),
  `golang.org/x/text` (NFKD), `coder/websocket`.

## Requirements

Go 1.26 or newer (the two latest Go releases are tested).

## Compatibility

| SDK | Node | REST envelope `version` | Wire protocol | Vectors |
|---|---|---|---|---|
| 0.1.x | 0.1.0 | 1.1.0 | 3.4.0 | v2 |

The `Spec…` constants export these values; the e2e test checks them against the node on the first call.

## Examples

Runnable programs in [`examples/`](examples): `go run ./examples/quickstart` and its siblings `subscribe`, `token`,
`validator`, `vault`. They read the `JANZEER_*` environment variables described in [CONTRIBUTING](CONTRIBUTING.md).

## Conformance

This SDK reproduces, byte for byte, the public test vectors every Janzeer SDK is tested against (key derivation,
addresses, every transaction preimage, hash and signature, the vault fixture) and passes the shared 14-step
end-to-end flow: <https://github.com/janzeerorg/janzeer-sdk-conformance>. See [docs/conformance.md](docs/conformance.md).

## Other SDKs

[TypeScript](https://github.com/janzeerorg/janzeer-sdk-ts) · [Dart / Flutter](https://github.com/janzeerorg/janzeer-sdk-dart) ·
[Kotlin / JVM / Android](https://github.com/janzeerorg/janzeer-sdk-kotlin) · [Python](https://github.com/janzeerorg/janzeer-sdk-python)

## Licence

Apache License 2.0 — see [LICENSE](LICENSE). Security reports: see [SECURITY](SECURITY.md).
