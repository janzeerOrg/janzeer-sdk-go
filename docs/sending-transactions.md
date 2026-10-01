# Sending transactions

Every transaction is built locally, signed locally and submitted as JSON. The node re-checks the hash, the signature,
the sender / public-key match, the nonce, the balance and the fees, and either accepts it into the mempool (HTTP 201 /
RPC `{hash, status: "PENDING"}`) or rejects it with a typed error.

```go
unsigned, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: me.Address(), Nonce: nonce}, To: to, Amount: "1.25"})
signed, err   := me.SignTx(unsigned)     // SignedTx: Hash, Signature, SenderPublicKey
_, err         = c.Submit(ctx, signed)   // REST
_, err         = r.Send(ctx, signed)     // or JSON-RPC — same body, same result
```

## Nonces

Each sender has a nonce that must be consecutive. `c.Nonce(ctx, address)` (or `r.GetAccount(ctx, address)` →
`NextNonce`) returns **committed nonce + pending count**, so you can pipeline several transactions before the first is
final: nonce `n`, `n+1`, `n+2`… A gap or a replay is refused with a `*janzeer.NonceMismatchError` (`Expected`, `Got`):
read `Expected`, rebuild, resend. Sending the *same* signed transaction twice is harmless.

## Network id — mainnet vs testnet

Every transaction is signed for one network: the network id is the first field of the signed bytes, so a transaction
built for `janzeer` (mainnet) is rejected by a `janzeer-testnet` node and vice versa. `Common.NetworkID` defaults to
`janzeer.NetworkID`; when your program can point at a testnet, read the id from the node once and pass it on:

```go
info, _ := c.Info(ctx)     // NetworkID, GenesisHash, Version, SyncStatus, Faucet, …
common := janzeer.Common{From: me.Address(), Nonce: nonce, NetworkID: info.NetworkID}
```

## Fees and limits

| Builder | Fee | Other rule |
|---|---|---|
| `NewTransfer` | ≥ `MinFee` (0.01) | amount ≥ `MinTransfer` (0.1) unless a memo is present; memo ≤ 256 UTF-8 bytes |
| `NewRegisterValidator` | exactly `ValidatorFee` (3) | deposit exactly `ValidatorDeposit` (2000), **non-refundable** |
| `NewExitValidator` | ≥ 0.01 | — |
| `NewTokenCreate` | exactly `TokenCreateFee` (5) | the sender must be a validator wallet |
| `NewTokenMint` / `Burn` / `SetCap` / `Transfer` | ≥ 0.01 | issuer-only for mint, burn and set-cap |

The builders fill the default fee and refuse locally what the node would refuse for these reasons, so you get an
error with a plain message instead of an HTTP 400. `r.EstimateFee(ctx, kind)` returns the same numbers from the node.
There is no fee market.

## Amounts

Coin amounts are **decimal strings** (`"1.25"`). Internally an amount is scaled by 10^8 to an integer
(`ToScaled("1.25")` → 125000000 as a `*big.Int`), which is what gets signed.

```go
janzeer.FormatJNZ("1.50000000", janzeer.FormatOptions{})            // "1.5 JNZ"
janzeer.FormatJNZ("1234.5", janzeer.FormatOptions{Group: true})     // "1,234.5 JNZ"
janzeer.ParseJNZ("1,234.5 JNZ")                                     // "1234.5"
janzeer.AddAmounts("0.1", "0.2")                                    // "0.30000000" — exact
janzeer.CompareAmounts("0.1", "0.10")                               // 0
```

## Validator registration and exit

```go
info, _ := c.Info(ctx)                                               // info.NodeKey: the NODE's public key
janzeer.NewRegisterValidator(janzeer.RegisterValidator{Common: common, ValidatorKey: info.NodeKey})   // fee 3, deposit 2000
janzeer.NewExitValidator(janzeer.ExitValidator{Common: next, ValidatorKey: info.NodeKey})             // removed at the next epoch
```

The key is the public key of a node you run, never the wallet's own key. The wallet that registers a node becomes its
*validator wallet*: block rewards go there, and only it can create tokens.

## Tokens (JZT-1)

Token amounts are **integer base units** as digit strings — the token's `Decimals` is display-only.

```go
create, _ := janzeer.NewTokenCreate(janzeer.Token{Common: common, Symbol: "DEMO", Name: "Demo", Decimals: 2, Cap: "1000000", Amount: "500000"})
tokenID   := create.Hash()                 // for CREATE the token id IS the transaction hash
janzeer.NewTokenMint(janzeer.Token{Common: common, TokenID: tokenID, Amount: "100", Recipient: to})
janzeer.NewTokenBurn(janzeer.Token{Common: common, TokenID: tokenID, Amount: "100"})
janzeer.NewTokenSetCap(janzeer.Token{Common: common, TokenID: tokenID, Cap: "2000000"})
janzeer.NewTokenTransfer(janzeer.Token{Common: common, TokenID: tokenID, Amount: "12345", Recipient: to})
```

## Waiting for finality

```go
ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
defer cancel()
view, err := rpc.WaitForFinality(ctx, r, signed.Hash)     // FINAL view with BlockHeight and Receipt
```

A transaction in a committed block is final; there is nothing to wait for beyond that. See
[json-rpc-and-subscriptions](json-rpc-and-subscriptions.md#finality).

## Signing elsewhere (hardware wallet, KMS)

```go
digest := unsigned.Hash()                                    // 32-byte hex to sign (RFC-6979 low-S DER expected)
signed := unsigned.WithSignature(base64DER, publicKeyHex)
```
