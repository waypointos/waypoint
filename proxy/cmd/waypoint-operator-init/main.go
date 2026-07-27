package main

import (
	"encoding/base64"
	"fmt"
	"os"

	"github.com/waypointos/waypoint/proxy/internal/operator"
)

func main() {
	op, err := operator.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, "create operator:", err)
		os.Exit(1)
	}
	seed, _ := op.Seed()
	fmt.Println("# Add this to proxy/.env (and Railway env vars):")
	fmt.Printf("OPERATOR_NKEY_SEED=%s\n", base64.StdEncoding.EncodeToString(seed))
	fmt.Println("# Public key (safe to commit; agent's identity.toml will carry it):")
	fmt.Printf("OPERATOR_PUBLIC_KEY=%s\n", op.PublicKey())
}
