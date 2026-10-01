# JSON-RPC and subscriptions

The node serves JSON-RPC 2.0 at `POST /rpc` and over WebSocket at `/rpc/ws` (the same methods, plus subscriptions).
`rpc.Client` (HTTP) and `rpc.WS` embed the same typed `rpc.Methods`; `Call` sends anything else.

```go
r := rpc.New("https://onion.janzeer.org")                    // "/rpc" appended automatically
r.GetAccount(ctx, addr); r.GetBalance(ctx, addr); r.GetNonce(ctx, addr)
r.GetTransactionByHash(ctx, hash)                            // Status FINAL | PENDING | UNKNOWN
r.GetTip(ctx); r.GetBlockByNumber(ctx, 12, true); r.GetBlocks(ctx, 1, 100, false)
r.ListValidators(ctx, 0, 0); r.GetActiveValidators(ctx); r.GetValidator(ctx, key)
r.EstimateFee(ctx, "transfer")                               // Fee, Rule "minimum" | "exact"
r.Send(ctx, signedTx)
r.GetInfo(ctx); r.GetChainSpec(ctx); r.GetStats(ctx); r.GetEpoch(ctx)   // raw JSON
var out json.RawMessage
r.Call(ctx, "janzeer_listTokens", map[string]any{"size": 20}, &out)     // any method
```

## Batches

```go
var nonce int64
var tip janzeer.BlockView
calls := []*rpc.BatchCall{
	{Method: "janzeer_getNonce", Params: map[string]any{"address": addr}, Out: &nonce},
	{Method: "janzeer_getTip", Out: &tip},
}
err := r.Batch(ctx, calls)          // err: the batch as a whole; calls[i].Err: one entry
```

Up to 50 requests per batch, 512 KiB per body. One failing entry does not fail the others.

## Finality

`r.WaitForFinalityOnce(ctx, hash, timeout)` is the node's long-poll (at most 60 s per call). The function
`rpc.WaitForFinality(ctx, r, hash)` chains calls until `ctx` ends (3 minutes when it has no deadline), tolerates a
dropped connection, and returns a `*janzeer.FinalityTimeoutError` (with the last view) on expiry. It accepts an
`rpc.Client` or an `rpc.WS`. Without JSON-RPC, `client.Client.WaitForFinality` polls the REST endpoints.

A `FINAL` view carries `BlockHash`, `BlockHeight` and `Receipt.Successful`. The chain has instant finality (2f+1 BFT
commit), so no confirmation counting exists anywhere in the API.

## WebSocket

```go
ws, err := rpc.Dial(ctx, "wss://onion.janzeer.org", nil)      // http(s) URLs are converted
defer ws.Close()
tip, err := ws.GetTip(ctx)                                    // every method works over the socket

from := int64(100)
blocks, err := ws.SubscribeNewBlocks(ctx, rpc.NewBlocksParams{FromHeight: &from, IncludeTransactions: true}, func(b *janzeer.BlockView) { … })
watch, err  := ws.SubscribeAddressActivity(ctx, []string{addr1, addr2}, func(ev *rpc.AddressActivity) { … })
err = watch.Unsubscribe(ctx)
```

- `newBlocks` replays committed blocks from `FromHeight`, then streams live ones — exactly once per height.
- `addressActivity` fires for every **final** transaction whose sender, recipient or token recipient is in the list
  (at most 1000 addresses, 16 subscriptions per socket).
- Handlers run one at a time on the reader goroutine: keep them fast, or hand the event to a channel. The node
  disconnects slow consumers (2 MiB send buffer, 5 s send timeout).
- **Reconnect**: on a dropped socket the client reconnects with exponential backoff (1 s up to 30 s) and re-issues
  every subscription; `Subscription.ID()` changes, handlers stay. `WSOptions{NoReconnect: true}` opts out;
  `WSOptions.OnEvent` reports `open`, `close`, `error`, `resubscribed`.

## Errors

JSON-RPC errors map to the same types as REST — see [errors](errors.md).
