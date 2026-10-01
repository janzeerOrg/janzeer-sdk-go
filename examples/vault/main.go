// Seal a recovery phrase under a password and open it again.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"

	janzeer "github.com/janzeerorg/janzeer-sdk-go"
	"github.com/janzeerorg/janzeer-sdk-go/vault"
)

func main() {
	phrase, err := janzeer.GenerateMnemonic(128)
	if err != nil {
		log.Fatal(err)
	}
	blob, err := vault.Encrypt(phrase, "correct horse battery staple")
	if err != nil {
		log.Fatal(err)
	}
	stored, _ := json.Marshal(blob)
	fmt.Println("vault:", len(stored), "bytes of JSON, version", blob.V)

	opened, err := vault.Decrypt(blob, "correct horse battery staple")
	if err != nil || opened != phrase {
		log.Fatal("round trip failed: ", err)
	}
	acct, _ := janzeer.AccountFromMnemonic(opened, "")
	fmt.Println("opened; address", acct.ChecksumAddress())
	if _, err := vault.Decrypt(blob, "wrong password"); errors.Is(err, vault.ErrVault) {
		fmt.Println("wrong password ->", err)
	}
}
