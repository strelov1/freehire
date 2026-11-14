// Command changes-webhook pushes the job change feed to the API customers who subscribed to
// it (POST /api/v1/webhooks/changes): signed, batched POSTs from each subscription's stored
// position in job_changes. One run claims every due subscription, delivers it until it has
// caught up with the feed (bounded per run) and exits; run it every minute.
//
// A customer endpoint failing is normal and does not fail the run: the worker records it on
// the subscription, retries by backoff and eventually disables it. The process exits non-zero
// only on an infrastructure error (database unreachable, a claim query failing).
//
// Needs only DATABASE_URL.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/changehook"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	stats, err := changehook.NewWorker(db.New(pool), changehook.NewSender()).Run(ctx)
	if err != nil {
		log.Printf("changes-webhook: %v", err)
		return 1
	}
	if stats.Subscriptions > 0 {
		log.Printf("changes-webhook: %d subscription(s), %d change(s) delivered, %d failed, %d disabled",
			stats.Subscriptions, stats.Changes, stats.Failed, stats.Disabled)
	}
	return 0
}
