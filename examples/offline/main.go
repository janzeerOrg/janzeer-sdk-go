// Sign a transfer offline and broadcast it later, in three separated steps:
// read the network id and the sender's nonce online (no key), sign on the
// offline machine (with the key, no network), then broadcast the signed body
// online (no key). Reads the JANZEER_* environment (see CONTRIBUTING.md).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
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

// offlineTx is what travels between the offline machine and the broadcaster:
// the transaction type and the signed body, exactly as SendRawTransaction
// takes them, plus the hash computed offline so step 3 can check that the
// node accepted the very transaction that was signed.
type offlineTx struct {
	Type string          `json:"type"`
	Hash string          `json:"hash"`
	Body json.RawMessage `json:"body"`
}

// Step 1, online, no key: read the network id and the sender's next nonce.
func step1(ctx context.Context, c *client.Client, sender string) (string, int64, error) {
	info, err := c.Info(ctx)
	if err != nil {
		return "", 0, err
	}
	nonce, err := c.Nonce(ctx, sender)
	if err != nil {
		return "", 0, err
	}
	fmt.Printf("step 1 (online, no key): network %s, next nonce %d for %s\n", info.NetworkID, nonce, sender)
	return info.NetworkID, nonce, nil
}

// Step 2, offline, with the key: build and sign the transfer, then write the
// type and the signed body as JSON to path. This step makes no network call.
//
// What must stay fresh is the nonce: it has to still be the account's next
// nonce when the transaction is broadcast. If the account sends anything else
// in between, the signed transaction is stale — go back to step 1 and sign
// again with the new nonce.
func step2(me *janzeer.Account, networkID string, nonce int64, to, path string) (string, error) {
	tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: me.Address(), Nonce: nonce, NetworkID: networkID}, To: to, Amount: "0.25", Data: "signed offline"})
	if err != nil {
		return "", err
	}
	signed, err := me.SignTx(tx)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(signed.Body())
	if err != nil {
		return "", err
	}
	out, err := json.MarshalIndent(offlineTx{Type: string(signed.Unsigned.Type), Hash: signed.Hash, Body: body}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o600); err != nil {
		return "", err
	}
	fmt.Printf("step 2 (offline, with the key): signed %s, wrote %s\n", signed.Hash, path)
	return signed.Hash, nil
}

// Step 3, online, no key: read the file back, broadcast the signed body and
// wait until it is final. The hash the node returns must equal the hash
// computed offline in step 2.
func step3(ctx context.Context, r *rpc.Client, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var f offlineTx
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	res, err := r.SendRawTransaction(ctx, f.Type, f.Body)
	if err != nil {
		return err
	}
	if res.Hash != f.Hash {
		return fmt.Errorf("node returned hash %s, but the offline signature was over %s", res.Hash, f.Hash)
	}
	final, err := rpc.WaitForFinality(ctx, r, res.Hash)
	if err != nil {
		return err
	}
	fmt.Printf("step 3 (online, no key): broadcast %s, final in block %d\n", res.Hash, *final.BlockHeight)
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	node := env("JANZEER_NODE_URL", "http://localhost:7019")
	c, r := client.New(node), rpc.New(env("JANZEER_RPC_URL", node))
	me, err := janzeer.AccountFromMnemonic(env("JANZEER_E2E_MNEMONIC", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"), "")
	if err != nil {
		log.Fatal(err)
	}
	to := env("JANZEER_E2E_RECIPIENT", "0x598b1301acef3baba6ce25e38dd17b723f7b98b1")
	path := env("JANZEER_OFFLINE_FILE", filepath.Join(os.TempDir(), "janzeer-offline-tx.json"))

	networkID, nonce, err := step1(ctx, c, me.Address())
	if err != nil {
		log.Fatal(err)
	}
	if _, err := step2(me, networkID, nonce, to, path); err != nil {
		log.Fatal(err)
	}
	if err := step3(ctx, r, path); err != nil {
		log.Fatal(err)
	}
}
