// Command auth-cleanup deletes bounded batches of expired v2 attempts,
// exchange codes, and recent-auth proofs. Schedule it every few minutes.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/auth/mobileauth"
	"github.com/strelov1/freehire/internal/worker"
)

func main() { worker.Main(run) }
func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	count, err := mobileauth.NewStore(pool).Cleanup(ctx, 1000)
	if err != nil {
		log.Printf("auth-cleanup: %v", err)
		return 1
	}
	log.Printf("auth-cleanup: deleted=%d", count)
	return 0
}
