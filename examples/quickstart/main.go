// Send a transfer and wait until it is final. Reads the JANZEER_* environment (see CONTRIBUTING.md).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
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

	info, err := c.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	balance, _ := c.Balance(ctx, me.Address())
	shown, _ := janzeer.FormatJNZ(balance, janzeer.FormatOptions{})
	fmt.Printf("node %s on network %s\n%s %s\n", info.Version, info.NetworkID, me.ChecksumAddress(), shown)

	nonce, err := c.Nonce(ctx, me.Address())
	if err != nil {
		log.Fatal(err)
	}
	tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: me.Address(), Nonce: nonce, NetworkID: info.NetworkID}, To: to, Amount: "1.25", Data: "hello from go"})
	if err != nil {
		log.Fatal(err)
	}
	signed, err := me.SignTx(tx)
	if err != nil {
		log.Fatal(err)
	}
	if _, err := c.Submit(ctx, signed); err != nil {
		log.Fatal(err)
	}
	fmt.Println("submitted", signed.Hash)
	final, err := rpc.WaitForFinality(ctx, r, signed.Hash)
	if err != nil {
		log.Fatal(err)
	}
	balance, _ = c.Balance(ctx, me.Address())
	shown, _ = janzeer.FormatJNZ(balance, janzeer.FormatOptions{})
	fmt.Println("final in block", *final.BlockHeight, "| balance now", shown)
}
