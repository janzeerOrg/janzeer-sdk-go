# Contributing

```bash
gofmt -l . && go vet ./... && staticcheck ./...     # format, vet, static analysis
go test -race ./...                                 # unit tests + conformance vectors (no network)
CGO_ENABLED=0 go build ./...                        # the module must build without cgo
```

## End-to-end test and examples

They need a running Janzeer network with the faucet-funded test wallet. With the private node repository checked out
next to the conformance kit:

```bash
../sdk_conformance/e2e/node-up.sh            # starts the 4-anchor dev net
eval "$(../sdk_conformance/e2e/node-up.sh --env)"
go test -tags e2e -run TestE2E -v .          # the 14-step SPEC flow
go run ./examples/quickstart
../sdk_conformance/e2e/node-down.sh
```

Against any other network set `JANZEER_NODE_URL`, `JANZEER_RPC_URL`, `JANZEER_WS_URL`, `JANZEER_E2E_MNEMONIC`
(a funded wallet) and `JANZEER_E2E_RECIPIENT` yourself.

## Conformance vectors

`internal/vectors/` is a vendored copy of `sdk_conformance/vectors/`. Never edit it by hand: `sdk_conformance/sync.sh
--regen` regenerates the vectors from the node and copies them here; `sync.sh --check` (and CI, through the
`SHA256SUMS` in that folder) fails on drift.

## Style

- Amounts are decimal strings at the API boundary; `float64` never appears in a public signature.
- A function that returns a value AND calls the transport assigns the error first (`err := …; return out, err`): Go
  does not order a variable read against a call inside one `return` statement.
- Every new node method gets a wrapper in `rpc` or `client`, a test against `httptest`, and a line in `CHANGELOG.md`.
- Public API says *validator*; the node's internal word *promoter* does not appear in it.
