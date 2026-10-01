// Create a JZT-1 token, then move some units. Token amounts are integer base units.
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
	// Only a VALIDATOR's wallet may create tokens (anti-spam).
	phrase := os.Getenv("JANZEER_E2E_VALIDATOR_MNEMONIC")
	if phrase == "" {
		fmt.Println("JANZEER_E2E_VALIDATOR_MNEMONIC is not set — token CREATE needs a validator wallet; skipping")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	node := env("JANZEER_NODE_URL", "http://localhost:7019")
	c, r := client.New(node), rpc.New(env("JANZEER_RPC_URL", node))
	issuer, err := janzeer.AccountFromMnemonic(phrase, "")
	if err != nil {
		log.Fatal(err)
	}
	holder := env("JANZEER_E2E_RECIPIENT", "0x598b1301acef3baba6ce25e38dd17b723f7b98b1")
	info, err := c.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	send := func(build func(janzeer.Common) (*janzeer.UnsignedTx, error)) *janzeer.TxView {
		nonce, err := c.Nonce(ctx, issuer.Address())
		if err != nil {
			log.Fatal(err)
		}
		tx, err := build(janzeer.Common{From: issuer.Address(), Nonce: nonce, NetworkID: info.NetworkID})
		if err != nil {
			log.Fatal(err)
		}
		signed, _ := issuer.SignTx(tx)
		if _, err := r.Send(ctx, signed); err != nil {
			log.Fatal(err)
		}
		view, err := rpc.WaitForFinality(ctx, r, signed.Hash)
		if err != nil {
			log.Fatal(err)
		}
		return view
	}

	created := send(func(c janzeer.Common) (*janzeer.UnsignedTx, error) {
		return janzeer.NewTokenCreate(janzeer.Token{Common: c, Symbol: "DEMO", Name: "Demo Token", Decimals: 2, Cap: "1000000", Amount: "500000"})
	})
	tokenID := created.Hash // for CREATE the token id is the transaction hash
	fmt.Println("token", tokenID, "created in block", *created.BlockHeight)
	moved := send(func(c janzeer.Common) (*janzeer.UnsignedTx, error) {
		return janzeer.NewTokenTransfer(janzeer.Token{Common: c, TokenID: tokenID, Amount: "12345", Recipient: holder})
	})
	fmt.Println("12345 units moved to", holder, "in block", *moved.BlockHeight)
}
