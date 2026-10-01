# Errors, nonces and rate limits

Errors are typed values. Use `errors.As` to branch on the type, never on message text.

| Type | When | Fields |
|---|---|---|
| `*janzeer.NetworkError` | node unreachable, non-JSON answer, socket closed, timeout | `Err` (wrapped) |
| `*janzeer.TxRejectedError` | the node refused a transaction | `Type`, `Transport` (`rest` / `rpc` / `ws`), `HTTPStatus`, `RPCCode` |
| `*janzeer.NonceMismatchError` | wrong nonce (also matches `*TxRejectedError`) | `Address`, `Expected`, `Got`, `Known` |
| `*janzeer.APIError` | other REST errors | `Status`, `Message`, `Type`, `Body` |
| `*janzeer.ValidationError` | HTTP 400 with a list of field messages (also matches `*APIError`) | `Messages` |
| `*janzeer.NotFoundError` | HTTP 404 (also matches `*APIError`) | |
| `*janzeer.NotSynchronizedError` | the node is still syncing (REST 400 / RPC -32002) | |
| `*janzeer.RateLimitedError` | HTTP 429 / RPC -32004 | `RetryAfter` |
| `*janzeer.RPCError` | other JSON-RPC errors | `Code`, `Data`; compare with `janzeer.RPCInvalidParams`, `RPCNotFound`, `RPCLimit`, `RPCMethodNotFound` |
| `*janzeer.FinalityTimeoutError` | the wait deadline passed | `Hash`, `Last` |

```go
var nonce *janzeer.NonceMismatchError
var rejected *janzeer.TxRejectedError
switch {
case errors.As(err, &nonce):                 // check the more specific type first
	fmt.Println("expected nonce", nonce.Expected)
case errors.As(err, &rejected):
	fmt.Println("refused:", rejected.Type)
case janzeer.IsRPCCode(err, janzeer.RPCNotFound):
	fmt.Println("no such block / transaction")
}
```

Local input mistakes (a bad address, a malformed amount) are plain errors returned by the builders, before anything
is sent.

## Rejection types

| `Type` | Meaning | What to do |
|---|---|---|
| `INVALID_NONCE` | nonce ≠ the sender's next nonce | refresh the nonce and rebuild |
| `INCORRECT_SIGNATURE` | the signature does not verify | you signed other bytes — check the fields and the network id |
| `INCORRECT_HASH` | `hash` ≠ double-SHA256 of the signed bytes | same |
| `INCORRECT_ADDRESS` | `senderAddress` is not the address of `senderPublicKey` | derive both from one `Account` |
| `INSUFFICIENT_ACTUAL_BALANCE` / `INSUFFICIENT_BALANCE` | not enough spendable balance for amount + fee | — |
| `INCORRECT_PROMOTER_KEY` / `ALREADY_PROMOTER` | validator key invalid / already registered | — |

## Nonce handling pattern

```go
func sendWithRetry(ctx context.Context, c *client.Client, me *janzeer.Account, build func(nonce int64) (*janzeer.UnsignedTx, error)) error {
	nonce, err := c.Nonce(ctx, me.Address())
	for attempt := 0; err == nil && attempt < 3; attempt++ {
		var tx *janzeer.UnsignedTx
		if tx, err = build(nonce); err != nil {
			return err
		}
		signed, _ := me.SignTx(tx)
		_, err = c.Submit(ctx, signed)
		var mismatch *janzeer.NonceMismatchError
		if errors.As(err, &mismatch) && mismatch.Known {
			nonce, err = mismatch.Expected, nil
			continue
		}
		return err
	}
	return err
}
```

Pipelining: after submitting nonce `n`, `c.Nonce` already answers `n+1`; you may keep sending. If a pending
transaction is dropped (6 h mempool expiry) the ones behind it become gaps — rebuild from `Expected`.

## Rate limits

Nodes throttle `POST /api/v1/transactions/**` and `/rpc` per client IP (default 20 requests/s, burst 40). Reads are
never limited. The clients retry **once** after a 429, honouring `Retry-After` (`NoRetryOnRateLimit` disables it);
resubmitting a transaction is safe because acceptance is idempotent by hash.
