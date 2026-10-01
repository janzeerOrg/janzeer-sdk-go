package janzeer

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// Account is a signing identity: private key -> public key -> address. The node never sees the private key.
// Printing an Account shows the address only.
type Account struct {
	priv    []byte
	pubHex  string
	address string
}

func newAccount(priv []byte) (*Account, error) {
	if !IsValidPrivateKey(hex.EncodeToString(priv)) {
		return nil, errors.New("janzeer: invalid secp256k1 private key")
	}
	pub := secp256k1.PrivKeyFromBytes(priv).PubKey().SerializeCompressed()
	addr, _ := PublicKeyToAddress(pub)
	return &Account{priv: append([]byte(nil), priv...), pubHex: hex.EncodeToString(pub), address: addr}, nil
}

// AccountFromMnemonic derives the wallet account of a BIP39 mnemonic (Janzeer seed, path m/0/0/0).
func AccountFromMnemonic(mnemonic, passphrase string) (*Account, error) {
	if !ValidateMnemonic(mnemonic) {
		return nil, errors.New("janzeer: invalid mnemonic (unknown word or bad checksum)")
	}
	return AccountFromSeed(MnemonicToSeed(NormalizeMnemonic(mnemonic), passphrase), DefaultPath)
}

// AccountFromSeed derives an account from a 64-byte seed and a non-hardened path.
func AccountFromSeed(seed []byte, path string) (*Account, error) {
	node, err := DerivePath(seed, path)
	if err != nil {
		return nil, err
	}
	return newAccount(node.Priv)
}

// AccountFromPrivateKey imports a raw private key (hex, optional 0x).
func AccountFromPrivateKey(privateKeyHex string) (*Account, error) {
	raw, err := HexToBytes(privateKeyHex)
	if err != nil {
		return nil, errors.New("janzeer: invalid secp256k1 private key")
	}
	return newAccount(raw)
}

// NewRandomAccount creates a fresh random key (not mnemonic-backed; prefer GenerateMnemonic for user wallets).
func NewRandomAccount() (*Account, error) {
	for {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		if a, err := newAccount(raw); err == nil {
			return a, nil
		}
	}
}

// Address is the canonical lowercase address.
func (a *Account) Address() string { return a.address }

// ChecksumAddress is the EIP-55 display form of the address.
func (a *Account) ChecksumAddress() string { return ToChecksumAddress(a.address) }

// PublicKeyHex is the compressed secp256k1 public key, hex (66 chars).
func (a *Account) PublicKeyHex() string { return a.pubHex }

// PrivateKeyHex returns the private key as hex. Handle with care: never log or send it.
func (a *Account) PrivateKeyHex() string { return hex.EncodeToString(a.priv) }

// Sign returns the RFC-6979 low-S DER signature (Base64) over a 32-byte digest given as hex.
func (a *Account) Sign(hashHex string) (string, error) {
	return SignHash(hashHex, hex.EncodeToString(a.priv))
}

// Verify checks a signature made by this account.
func (a *Account) Verify(hashHex, signatureBase64 string) bool {
	return VerifyHash(hashHex, signatureBase64, a.pubHex)
}

// SignTx hashes and signs a transaction. It fails if the transaction names another sender.
func (a *Account) SignTx(tx *UnsignedTx) (*SignedTx, error) {
	if tx.SenderAddress != a.address {
		return nil, fmt.Errorf("janzeer: tx sender %s is not this account (%s)", tx.SenderAddress, a.address)
	}
	sig, err := a.Sign(tx.Hash())
	if err != nil {
		return nil, err
	}
	return tx.WithSignature(sig, a.pubHex), nil
}

// String shows the address only.
func (a *Account) String() string { return "Account(" + a.address + ")" }

// GoString shows the address only, so %#v never prints the key.
func (a *Account) GoString() string { return a.String() }
