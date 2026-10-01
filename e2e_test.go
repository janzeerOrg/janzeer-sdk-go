//go:build e2e

package janzeer_test

// The conformance flow (janzeer-sdk-conformance e2e/SPEC.md) against a running network:
//   eval "$(../sdk_conformance/e2e/node-up.sh --env)" && go test -tags e2e -run TestE2E -v .

import (
	"context"
	"errors"
	"os"
	"regexp"
	"sync"
	"testing"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/client"
	"github.com/janzeerorg/janzeer-sdk-go/rpc"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func TestE2E(t *testing.T) {
	nodeURL := os.Getenv("JANZEER_NODE_URL")
	if nodeURL == "" {
		t.Skip("JANZEER_NODE_URL is not set")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	c, r := client.New(nodeURL), rpc.New(env("JANZEER_RPC_URL", nodeURL))
	recipient := env("JANZEER_E2E_RECIPIENT", "0x598b1301acef3baba6ce25e38dd17b723f7b98b1")
	must := func(err error, what string) {
		t.Helper()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
	}

	// 1 derive
	acct, err := janzeer.AccountFromMnemonic(env("JANZEER_E2E_MNEMONIC", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"), "")
	must(err, "derive")
	if os.Getenv("JANZEER_E2E_MNEMONIC") == "" && acct.Address() != "0x06e1c0fa9955a700876f8cb0acc7f13fba9fb8ba" {
		t.Fatal("canonical address")
	}

	// 2 version + network gate
	info, err := c.Info(ctx)
	must(err, "info")
	if info.Version != janzeer.SpecNode || c.LastEnvelope().Version != janzeer.SpecAPI {
		t.Fatalf("node %s api %s, SDK built for %s / %s", info.Version, c.LastEnvelope().Version, janzeer.SpecNode, janzeer.SpecAPI)
	}
	if want := os.Getenv("JANZEER_NETWORK_ID"); want != "" && info.NetworkID != want {
		t.Fatalf("network id %s", info.NetworkID)
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(info.GenesisHash) {
		t.Fatal("genesis hash")
	}
	common := func(nonce int64) janzeer.Common {
		return janzeer.Common{From: acct.Address(), Nonce: nonce, NetworkID: info.NetworkID}
	}

	// 3 account
	a0, err := r.GetAccount(ctx, acct.Address())
	must(err, "account")
	if cmp, _ := janzeer.CompareAmounts(a0.Balance.String(), "0"); cmp <= 0 {
		t.Fatal("the test wallet has no funds")
	}
	nonce := a0.NextNonce

	// 4 subscribe first, on ANOTHER node than the one the tx is sent to
	ws, err := rpc.Dial(ctx, env("JANZEER_WS_URL", nodeURL), nil)
	must(err, "ws dial")
	defer ws.Close()
	var mu sync.Mutex
	var seen []*rpc.AddressActivity
	sub, err := ws.SubscribeAddressActivity(ctx, []string{recipient}, func(ev *rpc.AddressActivity) { mu.Lock(); seen = append(seen, ev); mu.Unlock() })
	must(err, "subscribe")
	if !regexp.MustCompile(`^[0-9a-f]{16}$`).MatchString(sub.ID()) {
		t.Fatalf("subscription id %q", sub.ID())
	}

	// 5 build + sign
	utx, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: acct.Address(), Nonce: nonce, Fee: "0.01", NetworkID: info.NetworkID}, To: recipient, Amount: "1.25", Data: "sdk-e2e-go"})
	must(err, "build")
	tx, err := acct.SignTx(utx)
	must(err, "sign")
	if !acct.Verify(tx.Hash, tx.Signature) {
		t.Fatal("verify")
	}

	// 6 REST submit (201)
	echo, err := c.Submit(ctx, tx)
	must(err, "submit")
	if echo.Hash != tx.Hash {
		t.Fatal("echo hash")
	}

	// 7 RPC resubmit is idempotent
	again, err := r.Send(ctx, tx)
	must(err, "resubmit")
	if again.Hash != tx.Hash || again.Status != "PENDING" {
		t.Fatalf("resubmit: %+v", again)
	}
	if a, _ := r.GetAccount(ctx, acct.Address()); a.PendingCount > 1 {
		t.Fatal("pending count")
	}

	// 8 status
	if v, err := r.GetTransactionByHash(ctx, tx.Hash); err != nil || (v.Status != "PENDING" && v.Status != "FINAL") {
		t.Fatalf("status %v %v", v, err)
	}

	// 9 finality, via the WebSocket transport
	fctx, fcancel := context.WithTimeout(ctx, 2*time.Minute)
	fin, err := rpc.WaitForFinality(fctx, ws, tx.Hash)
	fcancel()
	must(err, "finality")
	if fin.Status != "FINAL" || fin.BlockHeight == nil || *fin.BlockHeight <= 0 || fin.Receipt == nil || !fin.Receipt.Successful {
		t.Fatalf("final view: %+v", fin)
	}

	// 10 exact balance (decimal strings, never floats)
	expected, _ := janzeer.SubAmounts(a0.Balance.String(), "1.25")
	expected, _ = janzeer.SubAmounts(expected, "0.01")
	bal, err := c.Balance(ctx, acct.Address())
	must(err, "balance")
	if got, _ := janzeer.SubAmounts(bal, "0"); got != expected {
		t.Fatalf("balance %s, want %s", got, expected)
	}

	// 11 notification
	var hit *rpc.AddressActivity
	for deadline := time.Now().Add(time.Minute); hit == nil && time.Now().Before(deadline); time.Sleep(250 * time.Millisecond) {
		mu.Lock()
		for _, ev := range seen {
			if ev.Transaction.Hash == tx.Hash {
				hit = ev
			}
		}
		mu.Unlock()
	}
	if hit == nil || hit.BlockHeight != *fin.BlockHeight {
		t.Fatalf("notification: %+v", hit)
	}
	must(sub.Unsubscribe(ctx), "unsubscribe")

	// 12 nonce gap -> NonceMismatchError (REST and RPC)
	gapTx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: common(nonce + 10), To: recipient, Amount: "1"})
	gap, _ := acct.SignTx(gapTx)
	var nm *janzeer.NonceMismatchError
	if _, err := c.Submit(ctx, gap); !errors.As(err, &nm) || nm.Type != "INVALID_NONCE" || nm.Got != nonce+10 {
		t.Fatalf("REST nonce gap: %v", err)
	}
	nm = nil
	if _, err := r.Send(ctx, gap); !errors.As(err, &nm) || nm.RPCCode != janzeer.RPCRejected {
		t.Fatalf("RPC nonce gap: %v", err)
	}

	// 13 bad signature -> TxRejectedError INCORRECT_SIGNATURE
	next, err := c.Nonce(ctx, acct.Address())
	must(err, "nonce")
	goodTx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: common(next), To: recipient, Amount: "1"})
	good, _ := acct.SignTx(goodTx)
	otherTx, _ := janzeer.NewTransfer(janzeer.Transfer{Common: common(next), To: recipient, Amount: "2"})
	var rej *janzeer.TxRejectedError
	if _, err := c.Submit(ctx, otherTx.WithSignature(good.Signature, acct.PublicKeyHex())); !errors.As(err, &rej) || rej.Type != "INCORRECT_SIGNATURE" {
		t.Fatalf("forged signature: %v", err)
	}

	// 14 bad address -> -32602
	if _, err := r.GetBalance(ctx, "0x12"); !janzeer.IsRPCCode(err, janzeer.RPCInvalidParams) {
		t.Fatalf("bad address: %v", err)
	}
}
