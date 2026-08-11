// Command push-receipts is the standalone Expo push-receipt polling worker.
// Expo's push-send response is only a ticket, not a final delivery outcome —
// a token that just went dead (app freshly uninstalled, permission freshly
// revoked) is only discoverable later, once Expo has actually attempted
// delivery through APNs/FCM, via its getReceipts endpoint. Each run claims a
// batch of tickets old enough for Expo to have an answer (at least 15
// minutes queued), checks them, and prunes any token whose receipt reports
// DeviceNotRegistered.
//
// It is a run-once-and-exit worker; schedule it on a cron every 15-20
// minutes. Needs only DATABASE_URL — Expo's API takes no credentials. An
// unscheduled worker is not a hard failure: the outbox just grows unchecked
// until it runs, since nothing else reads it.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pushnotify"
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

	store := pushnotify.NewQueriesStore(db.New(pool))
	notifier := pushnotify.NewExpoNotifier(store, store, store)

	if err := notifier.CheckReceipts(ctx); err != nil {
		log.Printf("push-receipts: %v", err)
		return 1
	}
	log.Printf("push-receipts: done")
	return 0
}
