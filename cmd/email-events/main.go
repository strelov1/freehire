// Command email-events drains the SES engagement queue into email_events and reduces events
// past retention to daily counts. Schedule it every few minutes. Without
// EMAIL_EVENTS_QUEUE_URL and AWS_REGION it only runs the retention pass.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailevents"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	store := db.New(pool)

	if queueURL := os.Getenv("EMAIL_EVENTS_QUEUE_URL"); queueURL != "" && cfg.AWSRegion != "" {
		src, err := emailevents.NewSQSSource(ctx, cfg.AWSRegion, queueURL)
		if err != nil {
			log.Printf("email-events: %v", err)
			return 1
		}
		stats, err := emailevents.Drain(ctx, src, store)
		log.Printf("email-events: recorded=%d ignored=%d failed=%d", stats.Recorded, stats.Ignored, stats.Failed)
		if err != nil {
			log.Printf("email-events: %v", err)
			return 1
		}
	}
	if err := emailevents.RollUp(ctx, store, time.Now()); err != nil {
		log.Printf("email-events: %v", err)
		return 1
	}
	return 0
}
