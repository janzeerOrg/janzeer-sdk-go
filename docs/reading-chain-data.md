# Reading chain data

`client.Client` wraps the `/api/v1` endpoints. Coin amounts decode into `janzeer.Amount` (the node's exact digits) or
are returned as decimal strings; nothing passes through a `float64`.

```go
c := client.New("https://onion.janzeer.org")      // "/api/v1/" is appended automatically
```

## Node and accounts

```go
info, err := c.Info(ctx)                 // NodeKey, NetworkID, GenesisHash, Version, SyncStatus, Faucet, …
gen, err  := c.Genesis(ctx)              // the network's public genesis document, raw JSON
c.LastEnvelope()                         // {Timestamp, Version} of the last response

balance, err := c.Balance(ctx, addr)     // spendable balance as a decimal string; "0" for an unknown address
nonce, err   := c.Nonce(ctx, addr)       // next nonce to sign with; 0 for an unknown address
```

`r.GetAccount(ctx, addr)` (JSON-RPC) returns balance, committed balance, both nonces and the pending count in one call.

## Transactions

```go
page, err := c.List(ctx, client.Transfers, client.ListOptions{Address: addr, Size: 20})
for _, t := range page.List {
	fmt.Println(t.Hash, *t.Amount, t.Final())
}
tx, err := c.Transaction(ctx, client.Transfers, hash)     // nil, nil when the node does not know the hash
rc, err := c.Receipt(ctx, hash)                           // nil, nil when unknown or still pending
```

Collections: `client.Transfers`, `ValidatorTxs`, `ExitValidators`, `TokenTxs`, `Rewards`. `ListOptions` pages
(`Page` 0-based, `Size` 1–100) and filters (`Address`, `Unconfirmed` for the mempool, `TokenID`, `NodeKey`). A
`janzeer.Transaction` names the common fields and keeps the complete object in `Raw`.

For one view of *any* transaction with its status (`FINAL | PENDING | UNKNOWN`), block and receipt, use
`r.GetTransactionByHash(ctx, hash)`.

## Validators

```go
all, err    := c.Validators(ctx, false, client.ListOptions{})   // every registered validator {Address, NodeKey}
active, err := c.Validators(ctx, true, client.ListOptions{})    // the current epoch's producer set
```

## Anything else

`c.Get(ctx, path, query, &out)` and `c.Post(ctx, path, body, &out)` reach any endpoint and decode the envelope payload
into `out`; `Get` reports `found == false` on a 404. Blocks, tokens and the richer views are on the JSON-RPC side:
`r.GetTip`, `r.GetBlockByNumber`, `r.GetBlocks`, and `r.Call` for the rest.

## Choosing REST or JSON-RPC

Both talk to the same services. REST suits explorers and dashboards (paged, cacheable GETs). JSON-RPC adds batches,
unified transaction and block views, the finality long-poll and the WebSocket feeds.
