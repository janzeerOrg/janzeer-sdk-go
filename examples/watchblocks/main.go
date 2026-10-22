// Watch newly finalized blocks over the WebSocket and print their height, hash and transaction count.
//
// It runs until three blocks arrive or 90 seconds pass. Like the other examples it reads JANZEER_* variables:
// JANZEER_WS_URL (default: JANZEER_NODE_URL, then http://localhost:7019) and, optionally,
// JANZEER_FROM_HEIGHT to replay from an earlier height instead of waiting for the next block.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/rpc"
)

const wantBlocks = 3

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var from *int64
	if raw := os.Getenv("JANZEER_FROM_HEIGHT"); raw != "" {
		height, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			log.Fatalf("JANZEER_FROM_HEIGHT: %v", err)
		}
		from = &height
	}

	ws, err := rpc.Dial(ctx, env("JANZEER_WS_URL", env("JANZEER_NODE_URL", "http://localhost:7019")), nil)
	if err != nil {
		log.Fatal(err)
	}
	defer ws.Close()

	blocks := make(chan struct{}, wantBlocks)
	sub, err := ws.SubscribeNewBlocks(ctx, rpc.NewBlocksParams{FromHeight: from}, func(b *janzeer.BlockView) {
		fmt.Println("block", b.Height, b.Hash, "txs", b.TransactionsCount)
		select {
		case blocks <- struct{}{}:
		default:
		}
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("subscribed", sub.ID(), "waiting for", wantBlocks, "blocks")

	for seen := 0; seen < wantBlocks; seen++ {
		select {
		case <-blocks:
		case <-ctx.Done():
			log.Fatal("timed out waiting for blocks")
		}
	}
	if err := sub.Unsubscribe(ctx); err != nil {
		log.Println("unsubscribe:", err)
	}
}
