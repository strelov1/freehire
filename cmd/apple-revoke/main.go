// Command apple-revoke drains durable native Sign in with Apple token
// revocations. It is run-once-and-exit and should be scheduled every minute.
package main

import (
	"context"
	"log"

	appleauth "github.com/strelov1/freehire/internal/auth/apple"
	"github.com/strelov1/freehire/internal/auth/applejobs"
	"github.com/strelov1/freehire/internal/worker"
)

func main() { worker.Main(run) }
func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	creds := cfg.OAuth["apple"]
	if cfg.AppleNativeClientID == "" {
		log.Print("apple-revoke: native Apple is not configured")
		return 1
	}
	client, err := appleauth.New(appleauth.Config{TeamID: creds.TeamID, KeyID: creds.KeyID, PrivateKeyPEM: creds.PrivateKey, ClientIDs: []string{cfg.AppleNativeClientID}})
	if err != nil {
		log.Printf("apple-revoke: %v", err)
		return 1
	}
	ring, err := appleauth.NewKeyRing(cfg.AppleGrantActiveKeyID, cfg.AppleGrantKeys)
	if err != nil {
		log.Printf("apple-revoke: %v", err)
		return 1
	}
	done, err := applejobs.New(pool, client, ring).Run(ctx, 100)
	if err != nil {
		log.Printf("apple-revoke: %v", err)
		return 1
	}
	log.Printf("apple-revoke: processed=%d", done)
	return 0
}
