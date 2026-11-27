// Command keygen generates WireGuard Curve25519 key pairs (and optional PSK).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/mywgclient/internal/crypto"
)

func main() {
	psk := flag.Bool("psk", false, "also generate a pre-shared key")
	flag.Parse()

	kp, err := crypto.GenerateKeyPair()
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("PrivateKey = %s\n", crypto.KeyToBase64(kp.Private))
	fmt.Printf("PublicKey  = %s\n", crypto.KeyToBase64(kp.Public))
	if *psk {
		k, err := crypto.GeneratePSK()
		if err != nil {
			fmt.Fprintf(os.Stderr, "psk: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("PresharedKey = %s\n", crypto.KeyToBase64(k))
	}
}
