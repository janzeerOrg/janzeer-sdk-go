package janzeer

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"strconv"
	"strings"

	"github.com/decred/dcrd/dcrec/secp256k1/v4"
	"github.com/decred/dcrd/dcrec/secp256k1/v4/ecdsa"
	"golang.org/x/crypto/pbkdf2"
	"golang.org/x/crypto/sha3"
	"golang.org/x/text/unicode/norm"
)

// The node's cryptography, byte for byte. It is BIP39/BIP32-SHAPED but uses Janzeer's own constants, so stock bip32
// or Ethereum libraries will NOT reproduce it: the PBKDF2 salt and the root HMAC key are both HDSalt, the path is
// m/0/0/0 (non-hardened), an address is the first 20 bytes of keccak256 over the COMPRESSED public key, and a
// signature is RFC-6979, low-S, DER, Base64, over the raw 32-byte digest. The conformance vectors pin all of it.

const (
	// HDSalt is used BOTH as the PBKDF2 salt base (instead of BIP39's "mnemonic") AND as the root HMAC key.
	HDSalt = "@_Janzeer_Blockchain_@"
	// DefaultPath is the only derivation path wallets use; all three levels are non-hardened.
	DefaultPath = "m/0/0/0"
)

var (
	// the order of the secp256k1 group
	curveN, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	addressRe = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
)

// HexToBytes decodes hex with an optional 0x prefix, either case.
func HexToBytes(s string) ([]byte, error) {
	if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		s = s[2:]
	}
	return hex.DecodeString(s)
}

func i64be(v int64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(v))
	return b
}

func ser32(v uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return b
}

// lenPrefixed is the node's length-prefixed string: int32(len) || utf8(s).
func lenPrefixed(s string) []byte { return append(ser32(uint32(len(s))), s...) }

// DoubleSHA256 is the transaction hash function.
func DoubleSHA256(data []byte) []byte {
	a := sha256.Sum256(data)
	b := sha256.Sum256(a[:])
	return b[:]
}

// HashPreimage is double-SHA256 as lowercase hex: what the node calls the transaction hash.
func HashPreimage(preimage []byte) string { return hex.EncodeToString(DoubleSHA256(preimage)) }

// Keccak256 is the Ethereum variant of Keccak, NOT NIST SHA3-256 (the padding differs).
func Keccak256(data []byte) []byte {
	h := sha3.NewLegacyKeccak256()
	h.Write(data)
	return h.Sum(nil)
}

// HDNode is an extended private key: a 32-byte private key and a 32-byte chain code.
type HDNode struct {
	Priv      []byte
	ChainCode []byte
}

// MnemonicToSeed is PBKDF2-HMAC-SHA512, 2048 rounds, 64 bytes, salt HDSalt + passphrase (NFKD inputs).
func MnemonicToSeed(mnemonic, passphrase string) []byte {
	return pbkdf2.Key([]byte(norm.NFKD.String(mnemonic)), []byte(HDSalt+norm.NFKD.String(passphrase)), 2048, 64, sha512.New)
}

// SeedToMasterKey is I = HMAC-SHA512(key = HDSalt, msg = seed): the left half is the key, the right the chain code.
func SeedToMasterKey(seed []byte) HDNode {
	m := hmac.New(sha512.New, []byte(HDSalt))
	m.Write(seed)
	i := m.Sum(nil)
	return HDNode{Priv: i[:32], ChainCode: i[32:]}
}

// DeriveChild is the standard BIP32 non-hardened private derivation (index < 2^31).
func DeriveChild(node HDNode, index uint32) (HDNode, error) {
	if index >= 0x80000000 {
		return HDNode{}, errors.New("janzeer: non-hardened index expected")
	}
	pub := secp256k1.PrivKeyFromBytes(node.Priv).PubKey().SerializeCompressed()
	m := hmac.New(sha512.New, node.ChainCode)
	m.Write(pub)
	m.Write(ser32(index))
	i := m.Sum(nil)
	child := new(big.Int).Add(new(big.Int).SetBytes(i[:32]), new(big.Int).SetBytes(node.Priv))
	child.Mod(child, curveN)
	return HDNode{Priv: child.FillBytes(make([]byte, 32)), ChainCode: i[32:]}, nil
}

// DerivePath derives a path like "m/0/0/0" (non-hardened segments only).
func DerivePath(seed []byte, path string) (HDNode, error) {
	parts := strings.Split(path, "/")
	if parts[0] != "m" {
		return HDNode{}, fmt.Errorf("janzeer: path must start with m/: %s", path)
	}
	node := SeedToMasterKey(seed)
	for _, p := range parts[1:] {
		if strings.HasSuffix(p, "'") || strings.HasSuffix(strings.ToLower(p), "h") {
			return HDNode{}, errors.New("janzeer: hardened derivation is not part of the Janzeer scheme")
		}
		i, err := strconv.ParseUint(p, 10, 32)
		if err != nil {
			return HDNode{}, fmt.Errorf("janzeer: bad path segment %q", p)
		}
		if node, err = DeriveChild(node, uint32(i)); err != nil {
			return HDNode{}, err
		}
	}
	return node, nil
}

// PublicKeyToAddress is the CANONICAL address: "0x" + lowercase hex of the first 20 bytes of keccak256(compressed key).
func PublicKeyToAddress(compressedPublicKey []byte) (string, error) {
	if len(compressedPublicKey) != 33 {
		return "", errors.New("janzeer: compressed (33-byte) public key expected")
	}
	return "0x" + hex.EncodeToString(Keccak256(compressedPublicKey)[:20]), nil
}

// ToChecksumAddress is the EIP-55 mixed-case display form. Presentation only: never sign or store it.
func ToChecksumAddress(address string) string {
	body := strings.TrimPrefix(strings.ToLower(address), "0x")
	digest := hex.EncodeToString(Keccak256([]byte(body)))
	out := []byte("0x" + body)
	for i := 0; i < len(body) && i < len(digest); i++ {
		if c := body[i]; c >= 'a' && c <= 'f' && digest[i] >= '8' {
			out[i+2] = c - 32
		}
	}
	return string(out)
}

// IsValidAddress checks shape and checksum the way the node does: all-lowercase, all-uppercase, or correct EIP-55.
func IsValidAddress(address string) bool {
	if !addressRe.MatchString(address) {
		return false
	}
	body := address[2:]
	if body == strings.ToLower(body) || body == strings.ToUpper(body) {
		return true
	}
	return ToChecksumAddress(address) == address
}

// NormalizeAddress returns the canonical lowercase form, or an error for an invalid address.
func NormalizeAddress(address string) (string, error) {
	if !IsValidAddress(address) {
		return "", fmt.Errorf("janzeer: invalid address: %s", address)
	}
	return strings.ToLower(address), nil
}

func parsePrivateKey(privateKeyHex string) (*secp256k1.PrivateKey, error) {
	raw, err := HexToBytes(privateKeyHex)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("janzeer: invalid secp256k1 private key")
	}
	k := new(big.Int).SetBytes(raw)
	if k.Sign() == 0 || k.Cmp(curveN) >= 0 {
		return nil, errors.New("janzeer: invalid secp256k1 private key")
	}
	return secp256k1.PrivKeyFromBytes(raw), nil
}

// IsValidPrivateKey reports whether the hex string is a valid secp256k1 scalar (32 bytes, 0 < k < n).
func IsValidPrivateKey(privateKeyHex string) bool {
	_, err := parsePrivateKey(privateKeyHex)
	return err == nil
}

// PrivateToPublic returns the compressed (33-byte) public key, hex.
func PrivateToPublic(privateKeyHex string) (string, error) {
	k, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(k.PubKey().SerializeCompressed()), nil
}

// SignHash signs a 32-byte digest (hex): RFC-6979 deterministic, low-S, DER, Base64. The digest is signed as-is.
func SignHash(hashHex, privateKeyHex string) (string, error) {
	digest, err := HexToBytes(hashHex)
	if err != nil || len(digest) != 32 {
		return "", errors.New("janzeer: a 32-byte digest is expected")
	}
	k, err := parsePrivateKey(privateKeyHex)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ecdsa.Sign(k, digest).Serialize()), nil
}

// VerifyHash verifies a Base64 DER signature over a digest (hex) with a compressed public key (hex). High-S is refused.
func VerifyHash(hashHex, signatureBase64, publicKeyHex string) bool {
	digest, err := HexToBytes(hashHex)
	if err != nil || len(digest) != 32 {
		return false
	}
	der, err := base64.StdEncoding.DecodeString(signatureBase64)
	if err != nil {
		return false
	}
	sig, err := ecdsa.ParseDERSignature(der)
	if err != nil {
		return false
	}
	s := sig.S()
	if s.IsOverHalfOrder() {
		return false
	}
	pubBytes, err := HexToBytes(publicKeyHex)
	if err != nil {
		return false
	}
	pub, err := secp256k1.ParsePubKey(pubBytes)
	if err != nil {
		return false
	}
	return sig.Verify(digest, pub)
}
