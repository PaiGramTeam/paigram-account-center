package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/PaiGramTeam/paigram-account-center/contracts/runtime/go/serviceticket"
)

type output struct {
	PrivateKeyPEM string `json:"private_key_pem"`
	PublicKeyPEM  string `json:"public_key_pem"`
}

func main() {
	privateKeyPEM, publicKeyPEM, err := serviceticket.GenerateKeyPairPEM()
	if err != nil {
		log.Fatal(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(output{
		PrivateKeyPEM: privateKeyPEM,
		PublicKeyPEM:  publicKeyPEM,
	}); err != nil {
		log.Fatal(err)
	}
}
