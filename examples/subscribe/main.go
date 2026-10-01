// Watch an address over the WebSocket, send it a transfer, and print the notification.
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	node := env("JANZEER_NODE_URL", "http://localhost:7019")
	c := client.New(node)
	me, err := janzeer.AccountFromMnemonic(env("JANZEER_E2E_MNEMONIC", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"), "")
	if err != nil {
		log.Fatal(err)
	}
	to := env("JANZEER_E2E_RECIPIENT", "0x598b1301acef3baba6ce25e38dd17b723f7b98b1")

	ws, err := rpc.Dial(ctx, env("JANZEER_WS_URL", node), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()
	got := make(chan *rpc.AddressActivity, 1)
	sub, err := ws.SubscribeAddressActivity(ctx, []string{to}, func(ev *rpc.AddressActivity) {
		select {
		case got <- ev:
		default:
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("subscribed", sub.ID())

	info, err := c.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	nonce, _ := c.Nonce(ctx, me.Address())
	tx, err := janzeer.NewTransfer(janzeer.Transfer{Common: janzeer.Common{From: me.Address(), Nonce: nonce, NetworkID: info.NetworkID}, To: to, Amount: "0.5"})
	if err != nil {
		log.Fatal(err)
	}
	signed, _ := me.SignTx(tx)
	if _, err := c.Submit(ctx, signed); err != nil {
		log.Fatal(err)
	}
	select {
	case ev := <-got:
		fmt.Println("block", ev.BlockHeight, "tx", ev.Transaction.Hash, "amount", *ev.Transaction.Amount)
	case <-ctx.Done():
		log.Fatal("no notification")
	}
	_ = sub.Unsubscribe(ctx)
}
