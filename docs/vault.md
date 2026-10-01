# Vault — a secret at rest

Package `vault` seals a mnemonic (or any string) under a password so a program can keep it in a file, a database row
or a keychain entry.

```go
blob, err := vault.Encrypt(phrase, password)        // *vault.Blob{V, Salt, IV, CT} — marshals to JSON as is
data, _   := json.Marshal(blob)

var stored vault.Blob
_ = json.Unmarshal(data, &stored)
phrase, err = vault.Decrypt(&stored, password)
if errors.Is(err, vault.ErrVault) {
	// wrong password or tampered blob
}
```

## Format (v1)

| Field | Value |
|---|---|
| key | PBKDF2-HMAC-SHA256, 250,000 rounds, 16-byte random `salt`, 32-byte key |
| cipher | AES-256-GCM, 12-byte random `iv`, `ct` = ciphertext ‖ 16-byte tag |
| encoding | all three fields base64 |

Blobs are interchangeable with the TypeScript, Dart, Kotlin and Python SDKs and with the Janzeer wallets (the
conformance kit's `vault-fixture.json` pins it).

A vault protects a phrase at rest; it does not make a weak password strong.
