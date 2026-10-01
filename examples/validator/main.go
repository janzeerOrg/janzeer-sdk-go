// Register (and later exit) a validator node. The 2000 JNZ deposit is NON-refundable: this example only builds and
// prints the transactions unless RUN_FOR_REAL=1.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/client"
)

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := client.New(env("JANZEER_NODE_URL", "http://localhost:7019"))
	wallet, err := janzeer.AccountFromMnemonic(env("JANZEER_E2E_MNEMONIC", "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"), "")
	if err != nil {
		log.Fatal(err)
	}
	info, err := c.Info(ctx)
	if err != nil {
		log.Fatal(err)
	}
	validatorKey := env("VALIDATOR_KEY", info.NodeKey) // the NODE's public key, never the wallet's
	nonce, err := c.Nonce(ctx, wallet.Address())
	if err != nil {
		log.Fatal(err)
	}

	reg, err := janzeer.NewRegisterValidator(janzeer.RegisterValidator{Common: janzeer.Common{From: wallet.Address(), Nonce: nonce, NetworkID: info.NetworkID}, ValidatorKey: validatorKey})
	if err != nil {
		log.Fatal(err)
	}
	register, _ := wallet.SignTx(reg)
	fmt.Printf("register %s… fee %s deposit %s\n  %v\n", validatorKey[:12], janzeer.ValidatorFee, janzeer.ValidatorDeposit, register.Body())
	ex, err := janzeer.NewExitValidator(janzeer.ExitValidator{Common: janzeer.Common{From: wallet.Address(), Nonce: nonce + 1, NetworkID: info.NetworkID}, ValidatorKey: validatorKey})
	if err != nil {
		log.Fatal(err)
	}
	exit, _ := wallet.SignTx(ex)
	fmt.Printf("exit (pipelined nonce)\n  %v\n", exit.Body())

	if os.Getenv("RUN_FOR_REAL") == "1" {
		echo, err := c.Submit(ctx, register)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("submitted", echo.Hash)
	} else {
		fmt.Println("dry run — set RUN_FOR_REAL=1 to submit the registration")
	}
}
