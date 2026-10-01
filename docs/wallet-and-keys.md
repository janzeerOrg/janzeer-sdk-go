# Wallet and keys

Janzeer wallets are BIP39-shaped but use the chain's own constants, so a stock bip32 or Ethereum library derives a
**different** key from the same phrase. Always derive with the SDK (or a client that passes the
[conformance vectors](conformance.md)).

## Create or import

```go
phrase, _  := janzeer.GenerateMnemonic(128)                 // 12 words (256 for 24)
account, _ := janzeer.AccountFromMnemonic(phrase, "")       // second argument: optional BIP39 passphrase
account.Address()          // canonical, lowercase — what the chain stores and what you sign with
account.ChecksumAddress()  // EIP-55 mixed case — for display and typo detection
account.PublicKeyHex()     // compressed secp256k1, 66 hex chars

janzeer.AccountFromPrivateKey("6dea…8efc")                              // raw key import
janzeer.AccountFromSeed(janzeer.MnemonicToSeed(phrase, ""), "m/0/1/0")  // another non-hardened path (advanced)
```

`ValidateMnemonic(phrase)` checks the word list and the checksum; `AccountFromMnemonic` returns an error for an
invalid phrase. Printing an `Account` shows the address only — the private key is never printed by the SDK.

## How derivation works (so you can audit it)

| Step | Janzeer | Standard BIP39/32 |
|---|---|---|
| seed | PBKDF2-HMAC-SHA512, 2048 rounds, salt `"@_Janzeer_Blockchain_@" + passphrase` | salt `"mnemonic" + passphrase` |
| master key | HMAC-SHA512 keyed with `"@_Janzeer_Blockchain_@"` | keyed with `"Bitcoin seed"` |
| path | `m/0/0/0`, all non-hardened | varies |
| address | `0x` + first 20 bytes of keccak256(**compressed** public key), lowercase | Ethereum hashes the uncompressed key |

The low-level functions are exported: `MnemonicToSeed`, `SeedToMasterKey`, `DeriveChild`, `DerivePath`,
`PublicKeyToAddress`, `Keccak256` (the Ethereum variant, not NIST SHA3-256).

## Addresses

```go
janzeer.IsValidAddress("0x06e1c0FA9955A700876f8cB0Acc7f13fBA9FB8BA")   // true  (correct EIP-55 casing)
janzeer.IsValidAddress("0x06E1c0fa9955a700876f8cb0acc7f13fba9fb8ba")   // false (mis-cased: likely a typo)
janzeer.NormalizeAddress("0x06e1c0FA…")                                // "0x06e1c0fa…", nil
```

The node accepts lowercase or correctly cased addresses and always answers lowercase. Sign and store the lowercase
form; show the checksum form.

## Signing

`account.Sign(hashHex)` produces an RFC-6979 deterministic, low-S, DER-encoded, Base64 signature over the 32-byte
digest — exactly what the node verifies. You rarely call it directly: `account.SignTx(unsigned)` hashes and signs a
transaction. A hardware wallet or KMS can sign `unsigned.Hash()` externally and attach it with
`unsigned.WithSignature(signature, publicKeyHex)`.

## Keeping the phrase at rest

Use the [vault](vault.md) package or your platform's secure storage. Never send the phrase or a private key to any
server: the node has no endpoint that would accept one.
