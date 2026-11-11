# Changelog

All notable changes to this module. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the module follows [Semantic Versioning](https://semver.org/).

## 0.1.4 — 2026-11-11

No API change. `SpecNode` is **0.1.13** (node hardening release after a full review: a peer is counted only under the key it proved in the handshake, a next-slot vote no longer discards the current slot, one reward transaction per block, format checks at intake, rate limit on `/rpc/ws`; API 1.1.0 / protocol 3.4.0 unchanged, rolling upgrade).

## 0.1.3 — 2026-11-11

Reliability fixes from a code review. Node pin unchanged (**0.1.12**, API 1.1.0 / protocol 3.4.0). No API change.

- **`client.WaitForFinality` over REST survives a dropped poll.** One transport error while polling (the transaction was already accepted) used to surface as a network error — which a caller can mistake for a failed send and resend. The REST branch now retries until the deadline, like the JSON-RPC branch.
- **WebSocket, resume after a reconnect.** A `newBlocks` stream now resumes right after the last block it delivered. It used to re-send the original `fromHeight`, so the node replayed every block again (thousands of duplicates on a long-lived stream), and a stream without `fromHeight` silently skipped the blocks committed during the outage. Blocks are delivered exactly once by height.
- **WebSocket, a failed resubscribe is retried.** A subscription whose re-creation failed (the socket dropped again, a call timed out) was removed for good with an `"error"` event as the only trace. It now stays registered and is re-created on the next attempt.

## 0.1.2 — 2026-11-09

No API change. `SpecNode` is **0.1.12** (node critical fix F-058: GraalVM native nodes could not sync across a block carrying a token transaction, fixed in 0.1.12 with a build-time guard; API 1.1.0 / protocol 3.4.0 unchanged).

- README: install with `go get github.com/janzeerorg/janzeer-sdk-go/...@v0.1.2` (or `go mod tidy` after importing) — `go get` of
  the root module alone leaves the `client` package's WebSocket dependency out of `go.sum`, and the first `go run` fails.

## 0.1.1 — 2026-11-01

No API change. `SpecNode` is **0.1.11** (node maintenance release: the `jz` command-line client ships in the node bundle;
`GET /info/version` now returns `version`, `apiVersion` and `protocolVersion` like `/info`; API 1.1.0 / protocol 3.4.0 unchanged).

## 0.1.0 — 2026-10-28

- `examples/watchblocks`: a `SubscribeNewBlocks` example that prints each finalized block's height, hash and
  transaction count.

First tagged version, for node 0.1.10 (API 1.1.0, protocol 3.4.0); the source has been public since 2026-10-01 and was
verified against every node release from 0.1.0 to 0.1.10.

- `janzeer`: BIP39 mnemonics, Janzeer seed and `m/0/0/0` derivation, addresses with the EIP-55 display form,
  RFC-6979 low-S signatures; exact amounts; builders for transfers, validator registration and exit and JZT-1 token
  operations; typed errors and result models.
- `client`: REST client. `rpc`: JSON-RPC over HTTP (with batches) and over WebSocket with `newBlocks` and
  `addressActivity` subscriptions and automatic resubscription; `WaitForFinality`.
- `vault`: PBKDF2-HMAC-SHA256 (250,000 rounds) + AES-256-GCM, interoperable with the other SDKs.
- Conformance: all vectors (format v2) and the 14-step end-to-end flow.
