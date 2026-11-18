// Command push-receipts is the standalone Expo push-receipt polling worker. Expo's push-send
// response is only a ticket; a dead token is discoverable later via getReceipts. Each run claims
// a batch of tickets at least 15 minutes old, checks them, and prunes any token whose receipt
// reports DeviceNotRegistered.
//
// It is a run-once-and-exit worker; schedule it every 15-20 minutes. Needs only DATABASE_URL.
// If unscheduled, the outbox just grows unchecked.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	store := pushnotify.NewQueriesStore(db.New(pool))
	notifier := pushnotify.NewExpoNotifier(store, store, store)

	if err := notifier.CheckReceipts(ctx); err != nil {
		log.Printf("push-receipts: %v", err)
		return 1
	}
	log.Printf("push-receipts: done")
	return 0
}
