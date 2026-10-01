// Package vault seals a secret under a password: PBKDF2-HMAC-SHA256 (250,000 rounds) -> AES-256-GCM.
//
// The blob {"v":1,"salt","iv","ct"} (base64 fields; ct = ciphertext || 16-byte tag) is byte-compatible with the
// TypeScript, Dart, Kotlin and Python SDK vaults (conformance fixture vault-fixture.json).
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

// Format constants.
const (
	Version    = 1
	Iterations = 250_000
)

// ErrVault is returned for a wrong password, a tampered blob or an unsupported version.
var ErrVault = errors.New("janzeer/vault: wrong password or corrupted vault")

// Blob is the sealed secret; it marshals to JSON as is.
type Blob struct {
	V    int    `json:"v"`
	Salt string `json:"salt"`
	IV   string `json:"iv"`
	CT   string `json:"ct"`
}

func gcm(password string, salt []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(pbkdf2.Key([]byte(password), salt, Iterations, 32, sha256.New))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Encrypt seals secret under password. Randomized: a fresh salt and IV on every call.
func Encrypt(secret, password string) (*Blob, error) {
	salt, iv := make([]byte, 16), make([]byte, 12)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}
	if _, err := rand.Read(iv); err != nil {
		return nil, err
	}
	aead, err := gcm(password, salt)
	if err != nil {
		return nil, err
	}
	b64 := base64.StdEncoding.EncodeToString
	return &Blob{V: Version, Salt: b64(salt), IV: b64(iv), CT: b64(aead.Seal(nil, iv, []byte(secret), nil))}, nil
}

// Decrypt opens a blob. It returns ErrVault (wrapped) for a wrong password or a tampered blob.
func Decrypt(blob *Blob, password string) (string, error) {
	if blob == nil || blob.V != Version {
		return "", fmt.Errorf("%w: unsupported vault version", ErrVault)
	}
	salt, err1 := base64.StdEncoding.DecodeString(blob.Salt)
	iv, err2 := base64.StdEncoding.DecodeString(blob.IV)
	ct, err3 := base64.StdEncoding.DecodeString(blob.CT)
	if err1 != nil || err2 != nil || err3 != nil || len(iv) != 12 {
		return "", ErrVault
	}
	aead, err := gcm(password, salt)
	if err != nil {
		return "", ErrVault
	}
	plain, err := aead.Open(nil, iv, ct, nil)
	if err != nil {
		return "", ErrVault
	}
	return string(plain), nil
}
