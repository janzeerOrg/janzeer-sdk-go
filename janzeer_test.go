package janzeer_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
)

const (
	addrA = "0x74d2bedc03ae5deb6fc63fbf7bb87e86f034b274"
	addrB = "0x598b1301acef3baba6ce25e38dd17b723f7b98b1"
	nodeK = "02406195b3b3eadde2d9f6cdb2ec288498b321c5b981b8aa44c13b7d4a3c3d7f51"
)

func TestAmounts(t *testing.T) {
	for in, want := range map[string]int64{"0.01": 1_000_000, "1.5": 150_000_000, "0.000000015": 2, "0.000000014": 1, "-2.5": -250_000_000, "3": 300_000_000, "92233720368.54775807": 9_223_372_036_854_775_807} {
		got, err := janzeer.ToScaled(in)
		if err != nil || got.Int64() != want {
			t.Errorf("ToScaled(%q) = %v (%v), want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "1e3", "+1", "1.", ".5", "1,5", "abc", "0x10"} {
		if _, err := janzeer.NormalizeAmount(bad); err == nil {
			t.Errorf("NormalizeAmount(%q) must fail", bad)
		}
	}
	if janzeer.IsValidAmount("1.123456789") || !janzeer.IsValidAmount("1.12345678") {
		t.Error("IsValidAmount")
	}
	if s, _ := janzeer.AddAmounts("0.1", "0.2"); s != "0.30000000" { // the sum a float gets wrong
		t.Errorf("0.1 + 0.2 = %s", s)
	}
	if s, _ := janzeer.SubAmounts("100000", "1.26"); s != "99998.74000000" {
		t.Errorf("sub = %s", s)
	}
	if c, _ := janzeer.CompareAmounts("1.0", "1"); c != 0 {
		t.Error("compare")
	}
	x, _ := janzeer.ToScaled("-0.00000001")
	if janzeer.FromScaled(x) != "-0.00000001" {
		t.Error("FromScaled")
	}
	for _, c := range []struct {
		in   string
		o    janzeer.FormatOptions
		want string
	}{
		{"1.50000000", janzeer.FormatOptions{}, "1.5 JNZ"},
		{"1234567.5", janzeer.FormatOptions{Group: true, NoTicker: true}, "1,234,567.5"},
		{"1.23456789", janzeer.FormatOptions{Decimals: 2}, "1.23 JNZ"}, // truncates, never rounds
		{"5", janzeer.FormatOptions{Decimals: 2, KeepZeros: true, NoTicker: true}, "5.00"},
		{"7.9", janzeer.FormatOptions{Decimals: janzeer.NoDecimals, NoTicker: true}, "7"},
	} {
		if got, err := janzeer.FormatJNZ(c.in, c.o); err != nil || got != c.want {
			t.Errorf("FormatJNZ(%q) = %q (%v), want %q", c.in, got, err, c.want)
		}
	}
	if s, err := janzeer.ParseJNZ(" 1,234.5 jnz "); err != nil || s != "1234.5" {
		t.Errorf("ParseJNZ = %q %v", s, err)
	}
}

func TestAmountKeepsTheNodesDigits(t *testing.T) {
	var v struct {
		A, B, C, D janzeer.Amount
	}
	if err := json.Unmarshal([]byte(`{"A":77499900.00200001,"B":"0.10000000","C":1E-2,"D":null}`), &v); err != nil {
		t.Fatal(err)
	}
	if v.A != "77499900.00200001" || v.B != "0.10000000" || v.C != "0.01" || v.D != "" { // a float64 would give …00200002
		t.Fatalf("%+v", v)
	}
	if err := json.Unmarshal([]byte(`{"A":"abc"}`), &v); err == nil {
		t.Fatal("garbage must fail")
	}
}

func TestBuilderRules(t *testing.T) {
	c := janzeer.Common{From: addrA}
	mustFail := func(name, part string, err error) {
		t.Helper()
		if err == nil || !strings.Contains(err.Error(), part) {
			t.Errorf("%s: want an error containing %q, got %v", name, part, err)
		}
	}
	_, err := janzeer.NewTransfer(janzeer.Transfer{Common: c, To: addrB, Amount: "0.05"})
	mustFail("small transfer", "unless the transfer carries a memo", err)
	if tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: c, To: addrB, Amount: "0", Data: "note"}); err != nil || tx.Fields["data"] != "note" {
		t.Errorf("memo transfer: %v", err)
	}
	_, err = janzeer.NewTransfer(janzeer.Transfer{Common: c, To: addrB, Amount: "1", Data: strings.Repeat("x", 257)})
	mustFail("long memo", "memo exceeds", err)
	_, err = janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: addrA, Fee: "0.001"}, To: addrB, Amount: "1"})
	mustFail("low fee", "fee", err)
	_, err = janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: addrA, Nonce: -1}, To: addrB, Amount: "1"})
	mustFail("negative nonce", "nonce", err)
	_, err = janzeer.NewTransfer(janzeer.Transfer{Common: c, To: "0x12", Amount: "1"})
	mustFail("bad address", "invalid address", err)
	_, err = janzeer.NewRegisterValidator(janzeer.RegisterValidator{Common: janzeer.Common{From: addrA, Fee: "4"}, ValidatorKey: nodeK})
	mustFail("validator fee", "exactly 3", err)
	_, err = janzeer.NewRegisterValidator(janzeer.RegisterValidator{Common: c, ValidatorKey: nodeK, Amount: "1999"})
	mustFail("validator deposit", "exactly 2000", err)
	_, err = janzeer.NewExitValidator(janzeer.ExitValidator{Common: c, ValidatorKey: "abc"})
	mustFail("validator key", "validator key", err)
	_, err = janzeer.NewTokenCreate(janzeer.Token{Common: janzeer.Common{From: addrA, Fee: "0.01"}, Symbol: "X", Name: "X"})
	mustFail("token create fee", "exactly 5", err)
	_, err = janzeer.NewTokenMint(janzeer.Token{Common: c, TokenID: strings.Repeat("a", 64), Amount: "1.5"})
	mustFail("token units", "base units", err)
	_, err = janzeer.NewTokenTransfer(janzeer.Token{Common: c, TokenID: strings.Repeat("a", 64), Amount: "5"})
	mustFail("token recipient", "recipient is required", err)

	// EIP-55 input is normalized; a mis-cased address is refused
	tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: janzeer.ToChecksumAddress(addrA), Timestamp: 1}, To: janzeer.ToChecksumAddress(addrB), Amount: "1"})
	if err != nil || tx.SenderAddress != addrA || tx.Fields["recipientAddress"] != addrB {
		t.Errorf("normalization: %v", err)
	}
	swapped := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'f' {
			return r - 32
		} else if r >= 'A' && r <= 'F' {
			return r + 32
		}
		return r
	}, janzeer.ToChecksumAddress(addrB)[2:])
	if janzeer.IsValidAddress("0x" + swapped) {
		t.Error("a mis-cased address must be invalid")
	}
}

func TestAccountSafety(t *testing.T) {
	a, err := janzeer.NewRandomAccount()
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{fmt.Sprint(a), fmt.Sprintf("%v", a), fmt.Sprintf("%+v", a), fmt.Sprintf("%#v", a)} {
		if strings.Contains(s, a.PrivateKeyHex()) || !strings.Contains(s, a.Address()) {
			t.Fatalf("printing an account must show the address only: %s", s)
		}
	}
	tx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: addrA}, To: addrB, Amount: "1"})
	if _, err := a.SignTx(tx); err == nil || !strings.Contains(err.Error(), "not this account") {
		t.Fatalf("signing for another sender must fail: %v", err)
	}
	for strength, n := range map[int]int{128: 12, 256: 24} {
		m, err := janzeer.GenerateMnemonic(strength)
		if err != nil || len(strings.Fields(m)) != n || !janzeer.ValidateMnemonic(m) {
			t.Fatalf("mnemonic %d: %v", strength, err)
		}
	}
	good := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	if !janzeer.ValidateMnemonic("  ABANDON "+good[8:]) || janzeer.ValidateMnemonic(strings.Replace(good, "about", "abandon", 1)) || janzeer.ValidateMnemonic("hello world") {
		t.Fatal("mnemonic validation")
	}
	if len(janzeer.Wordlist()) != 2048 {
		t.Fatal("word list")
	}
}

func TestRESTErrorMapping(t *testing.T) {
	env := func(payload string) []byte {
		return []byte(`{"timestamp":1,"version":"1.1.0","payload":` + payload + `}`)
	}
	var nm *janzeer.NonceMismatchError
	var rej *janzeer.TxRejectedError
	err := janzeer.RESTErrorFrom(400, env(`{"status":400,"message":"Invalid nonce for `+addrA+`: expected 3, got 13","type":"INVALID_NONCE"}`), "")
	if !errors.As(err, &nm) || !nm.Known || nm.Expected != 3 || nm.Got != 13 || nm.HTTPStatus != 400 || !errors.As(err, &rej) {
		t.Fatalf("nonce: %#v", err)
	}
	err = janzeer.RESTErrorFrom(400, env(`{"status":400,"message":"Incorrect signature","type":"INCORRECT_SIGNATURE"}`), "")
	if !errors.As(err, &rej) || rej.Type != "INCORRECT_SIGNATURE" || rej.Transport != "rest" || errors.As(err, &nm) && nm == nil {
		t.Fatalf("rejected: %#v", err)
	}
	var ve *janzeer.ValidationError
	err = janzeer.RESTErrorFrom(400, env(`[{"message":"amount: must not be null"},{"message":"fee: too small"}]`), "")
	if !errors.As(err, &ve) || len(ve.Messages) != 2 || ve.Messages[1] != "fee: too small" {
		t.Fatalf("validation: %#v", err)
	}
	var ns *janzeer.NotSynchronizedError
	if err = janzeer.RESTErrorFrom(400, env(`{"message":"Blockchain is synchronizing"}`), ""); !errors.As(err, &ns) {
		t.Fatalf("sync: %#v", err)
	}
	var nf *janzeer.NotFoundError
	var api *janzeer.APIError
	if err = janzeer.RESTErrorFrom(404, env(`{"message":"no such block"}`), ""); !errors.As(err, &nf) || !errors.As(err, &api) || api.Status != 404 {
		t.Fatalf("404: %#v", err)
	}
	var rl *janzeer.RateLimitedError
	if err = janzeer.RESTErrorFrom(429, []byte(`{"status":429,"message":"slow down"}`), "2"); !errors.As(err, &rl) || rl.RetryAfter.Seconds() != 2 || rl.Message != "slow down" {
		t.Fatalf("429 (flat body): %#v", err)
	}
	if err = janzeer.RESTErrorFrom(500, []byte(`<html>`), ""); !errors.As(err, &api) || api.Status != 500 {
		t.Fatalf("500: %#v", err)
	}
}

func TestRPCErrorMapping(t *testing.T) {
	var nm *janzeer.NonceMismatchError
	err := janzeer.RPCErrorFrom(-32001, "Invalid nonce for "+addrA+": expected 1, got 11", json.RawMessage(`{"type":"INVALID_NONCE"}`), "ws")
	if !errors.As(err, &nm) || nm.Expected != 1 || nm.RPCCode != janzeer.RPCRejected || nm.Transport != "ws" {
		t.Fatalf("%#v", err)
	}
	var rej *janzeer.TxRejectedError
	if err = janzeer.RPCErrorFrom(-32001, "Incorrect signature", json.RawMessage(`{"type":"INCORRECT_SIGNATURE"}`), "rpc"); !errors.As(err, &rej) || rej.Type != "INCORRECT_SIGNATURE" {
		t.Fatalf("%#v", err)
	}
	if err = janzeer.RPCErrorFrom(-32602, "bad", nil, "rpc"); !janzeer.IsRPCCode(err, janzeer.RPCInvalidParams) || janzeer.IsRPCCode(err, janzeer.RPCNotFound) {
		t.Fatalf("%#v", err)
	}
	var ns *janzeer.NotSynchronizedError
	if err = janzeer.RPCErrorFrom(-32002, "", nil, "rpc"); !errors.As(err, &ns) {
		t.Fatalf("%#v", err)
	}
}
