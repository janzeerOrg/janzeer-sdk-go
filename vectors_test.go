package janzeer_test

import (
	"encoding/hex"
	"encoding/json"
	"testing"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/internal/vectors"
)

// The conformance vectors: every value the node produces must be reproduced byte for byte.

type txVec struct {
	Timestamp        int64   `json:"timestamp"`
	Fee              string  `json:"fee"`
	Nonce            int64   `json:"nonce"`
	SenderAddress    string  `json:"senderAddress"`
	PublicKey        string  `json:"publicKey"`
	RecipientAddress string  `json:"recipientAddress"`
	Amount           *string `json:"amount"`
	Data             *string `json:"data"`
	PromoterKey      string  `json:"promoterKey"`
	TokenID          string  `json:"tokenId"`
	Symbol           string  `json:"symbol"`
	Name             string  `json:"name"`
	Decimals         int     `json:"decimals"`
	Cap              *string `json:"cap"`
	Recipient        *string `json:"recipient"`
	PreimageHex      string  `json:"preimageHex"`
	Hash             string  `json:"hash"`
	Signature        string  `json:"signature"`
}

type vec struct {
	VectorsVersion int                                                                            `json:"vectorsVersion"`
	NetworkID      string                                                                         `json:"networkId"`
	HD             struct{ Mnemonic, Passphrase, SeedHex, Path, PrivHex, PubHex, Address string } `json:"hd"`
	RawKey         struct{ PrivHex, PubHex, Address string }                                      `json:"rawKey"`
	Addresses      []struct{ Lower, Checksum string }                                             `json:"addresses"`
	Scaled         []struct {
		In  string `json:"in"`
		Out int64  `json:"out"`
	} `json:"scaled"`
	TransferTx       txVec `json:"transferTx"`
	TransferNoMemoTx txVec `json:"transferNoMemoTx"`
	PromoterTx       txVec `json:"promoterTx"`
	ExitPromoterTx   txVec `json:"exitPromoterTx"`
	TokenCreateTx    txVec `json:"tokenCreateTx"`
	TokenTransferTx  txVec `json:"tokenTransferTx"`
}

func load(t *testing.T) vec {
	t.Helper()
	var v vec
	if err := json.Unmarshal(vectors.WalletParity, &v); err != nil {
		t.Fatal(err)
	}
	if v.VectorsVersion != janzeer.SpecVectors || v.NetworkID != janzeer.NetworkID {
		t.Fatalf("unexpected vectors: version %d network %s", v.VectorsVersion, v.NetworkID)
	}
	return v
}

func s(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func TestHDDerivation(t *testing.T) {
	v := load(t)
	seed := janzeer.MnemonicToSeed(v.HD.Mnemonic, v.HD.Passphrase)
	if hex.EncodeToString(seed) != v.HD.SeedHex {
		t.Fatal("seed")
	}
	node, err := janzeer.DerivePath(seed, v.HD.Path)
	if err != nil || hex.EncodeToString(node.Priv) != v.HD.PrivHex {
		t.Fatal("derived key", err)
	}
	a, err := janzeer.AccountFromMnemonic(v.HD.Mnemonic, "")
	if err != nil || a.PrivateKeyHex() != v.HD.PrivHex || a.PublicKeyHex() != v.HD.PubHex || a.Address() != v.HD.Address {
		t.Fatalf("account from mnemonic: %v %v", a, err)
	}
	ent, err := janzeer.MnemonicToEntropy(v.HD.Mnemonic)
	if err != nil {
		t.Fatal(err)
	}
	if m, _ := janzeer.MnemonicFromEntropy(ent); m != v.HD.Mnemonic {
		t.Fatal("mnemonic round trip")
	}
}

func TestRawKeyAndChecksums(t *testing.T) {
	v := load(t)
	a, err := janzeer.AccountFromPrivateKey(v.RawKey.PrivHex)
	if err != nil || a.PublicKeyHex() != v.RawKey.PubHex || a.Address() != v.RawKey.Address {
		t.Fatal("raw key", err)
	}
	for _, x := range v.Addresses {
		if janzeer.ToChecksumAddress(x.Lower) != x.Checksum || !janzeer.IsValidAddress(x.Checksum) {
			t.Fatalf("checksum of %s", x.Lower)
		}
	}
}

func TestScaledAmounts(t *testing.T) {
	for _, row := range load(t).Scaled {
		got, err := janzeer.ToScaled(row.In)
		if err != nil || got.Int64() != row.Out {
			t.Fatalf("ToScaled(%q) = %v, want %d (%v)", row.In, got, row.Out, err)
		}
	}
}

func check(t *testing.T, name string, tx *janzeer.UnsignedTx, err error, want txVec, key string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if got := hex.EncodeToString(tx.Preimage()); got != want.PreimageHex {
		t.Fatalf("%s preimage\n got %s\nwant %s", name, got, want.PreimageHex)
	}
	if tx.Hash() != want.Hash {
		t.Fatalf("%s hash", name)
	}
	acct, _ := janzeer.AccountFromPrivateKey(key)
	signed, err := acct.SignTx(tx)
	if err != nil || signed.Signature != want.Signature { // exact: RFC-6979 + low-S, or this fails
		t.Fatalf("%s signature\n got %s\nwant %s (%v)", name, signed.Signature, want.Signature, err)
	}
	if !janzeer.VerifyHash(want.Hash, want.Signature, want.PublicKey) {
		t.Fatalf("%s verify", name)
	}
}

func TestTransactions(t *testing.T) {
	v := load(t)
	key := v.RawKey.PrivHex
	common := func(x txVec) janzeer.Common {
		return janzeer.Common{From: x.SenderAddress, Nonce: x.Nonce, Fee: x.Fee, Timestamp: x.Timestamp}
	}
	for name, x := range map[string]txVec{"transfer": v.TransferTx, "transferNoMemo": v.TransferNoMemoTx} {
		tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: common(x), To: x.RecipientAddress, Amount: s(x.Amount), Data: s(x.Data)})
		check(t, name, tx, err, x, key)
		signed, _ := mustAccount(t, key).SignTx(tx)
		if _, has := signed.Body()["data"]; has != (x.Data != nil) {
			t.Fatalf("%s: data presence in the body", name)
		}
	}
	x := v.PromoterTx
	tx, err := janzeer.NewRegisterValidator(janzeer.RegisterValidator{Common: common(x), ValidatorKey: x.PromoterKey, Amount: s(x.Amount)})
	check(t, "registerValidator", tx, err, x, key)
	x = v.ExitPromoterTx
	tx, err = janzeer.NewExitValidator(janzeer.ExitValidator{Common: common(x), ValidatorKey: x.PromoterKey})
	check(t, "exitValidator", tx, err, x, key)
	x = v.TokenCreateTx
	tx, err = janzeer.NewTokenCreate(janzeer.Token{Common: common(x), Symbol: x.Symbol, Name: x.Name, Decimals: x.Decimals, Cap: s(x.Cap), Amount: s(x.Amount)})
	check(t, "tokenCreate", tx, err, x, key)
	x = v.TokenTransferTx
	tx, err = janzeer.NewTokenTransfer(janzeer.Token{Common: common(x), TokenID: x.TokenID, Amount: s(x.Amount), Recipient: s(x.Recipient)})
	check(t, "tokenTransfer", tx, err, x, key)
}

func mustAccount(t *testing.T, key string) *janzeer.Account {
	t.Helper()
	a, err := janzeer.AccountFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return a
}
