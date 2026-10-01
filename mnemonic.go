package janzeer

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"errors"
	"math/big"
	"strings"
	"sync"
)

// BIP39 English mnemonics (standard word list and checksum). Only the SEED derivation is Janzeer-specific.

//go:embed bip39_english.txt
var bip39English string

var (
	wordsOnce sync.Once
	words     []string
	wordIndex map[string]int
)

func loadWords() {
	wordsOnce.Do(func() {
		words = strings.Fields(bip39English)
		wordIndex = make(map[string]int, len(words))
		for i, w := range words {
			wordIndex[w] = i
		}
	})
}

// Wordlist returns a copy of the 2048 English words.
func Wordlist() []string {
	loadWords()
	return append([]string(nil), words...)
}

// NormalizeMnemonic normalizes whitespace and case so equivalent phrases derive the same seed.
func NormalizeMnemonic(mnemonic string) string {
	return strings.Join(strings.Fields(strings.ToLower(mnemonic)), " ")
}

// MnemonicFromEntropy turns 16, 20, 24, 28 or 32 bytes of entropy into a mnemonic.
func MnemonicFromEntropy(entropy []byte) (string, error) {
	bits := len(entropy) * 8
	if bits < 128 || bits > 256 || bits%32 != 0 {
		return "", errors.New("janzeer: entropy must be 16, 20, 24, 28 or 32 bytes")
	}
	loadWords()
	checksumBits := uint(bits / 32)
	sum := sha256.Sum256(entropy)
	v := new(big.Int).SetBytes(entropy)
	v.Lsh(v, checksumBits)
	v.Or(v, big.NewInt(int64(sum[0]>>(8-checksumBits))))
	count := (bits + int(checksumBits)) / 11
	out := make([]string, count)
	mask := big.NewInt(0x7FF)
	for i := count - 1; i >= 0; i-- {
		out[i] = words[new(big.Int).And(v, mask).Int64()]
		v.Rsh(v, 11)
	}
	return strings.Join(out, " "), nil
}

// MnemonicToEntropy returns the entropy of a mnemonic, or an error for an unknown word, a bad length or a bad checksum.
func MnemonicToEntropy(mnemonic string) ([]byte, error) {
	parts := strings.Split(NormalizeMnemonic(mnemonic), " ")
	switch len(parts) {
	case 12, 15, 18, 21, 24:
	default:
		return nil, errors.New("janzeer: a mnemonic has 12, 15, 18, 21 or 24 words")
	}
	loadWords()
	v := new(big.Int)
	for _, w := range parts {
		i, ok := wordIndex[w]
		if !ok {
			return nil, errors.New("janzeer: unknown word in mnemonic")
		}
		v.Lsh(v, 11)
		v.Or(v, big.NewInt(int64(i)))
	}
	checksumBits := uint(len(parts) * 11 / 33)
	bits := len(parts)*11 - int(checksumBits)
	checksum := new(big.Int).And(v, big.NewInt(int64(1)<<checksumBits-1)).Int64()
	entropy := new(big.Int).Rsh(v, checksumBits).FillBytes(make([]byte, bits/8))
	sum := sha256.Sum256(entropy)
	if int64(sum[0]>>(8-checksumBits)) != checksum {
		return nil, errors.New("janzeer: bad mnemonic checksum")
	}
	return entropy, nil
}

// GenerateMnemonic returns a fresh mnemonic from the OS random source: strength 128 (12 words) … 256 (24 words).
func GenerateMnemonic(strength int) (string, error) {
	if strength < 128 || strength > 256 || strength%32 != 0 {
		return "", errors.New("janzeer: strength must be 128, 160, 192, 224 or 256")
	}
	entropy := make([]byte, strength/8)
	if _, err := rand.Read(entropy); err != nil {
		return "", err
	}
	return MnemonicFromEntropy(entropy)
}

// ValidateMnemonic checks word-list membership and the checksum. Client-side only: the node never sees a mnemonic.
func ValidateMnemonic(mnemonic string) bool {
	_, err := MnemonicToEntropy(mnemonic)
	return err == nil
}
