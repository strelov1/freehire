// Command mail-ingest is the hosted-mailbox inbound worker. SES receives mail for
// the hosted-mailbox domain, stores the raw MIME in S3, and notifies SNS → SQS;
// this daemon long-polls the queue, parses each message, resolves the recipient
// to the owning user, and stores it in the unified mail store. Unlike the
// run-once cron workers it is long-lived (SES delivery is push-driven), so it
// loops until SIGTERM.
//
// It is gated on config: without MAILBOX_DOMAIN / AWS_REGION / MAIL_INBOUND_QUEUE_URL
// it exits cleanly (nothing to drain). AWS credentials come from the default chain
// (instance/SSO role), never app config.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailingest"
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

	domain := cfg.MailboxDomain
	region := os.Getenv("AWS_REGION")
	queueURL := os.Getenv("MAIL_INBOUND_QUEUE_URL")
	bucket := os.Getenv("MAIL_INBOUND_BUCKET")
	if domain == "" || region == "" || queueURL == "" {
		log.Print("mail-ingest: not configured (MAILBOX_DOMAIN / AWS_REGION / MAIL_INBOUND_QUEUE_URL) — nothing to do")
		return 0
	}

	src, err := mailingest.NewSESSource(ctx, region, queueURL, bucket)
	if err != nil {
		log.Printf("mail-ingest: ses source: %v", err)
		return 1
	}
	store := mailingest.NewDBStore(db.New(pool))

	log.Printf("mail-ingest: draining %s into hosted mailboxes on %s", queueURL, domain)
	mailingest.NewWorker(src, store, domain).Run(ctx) // blocks until SIGTERM
	return 0
}
