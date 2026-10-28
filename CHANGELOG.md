# Changelog

All notable changes to this module. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/)
and the module follows [Semantic Versioning](https://semver.org/).

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
