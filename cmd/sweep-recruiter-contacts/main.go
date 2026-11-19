// Command sweep-recruiter-contacts deletes cached recruiter contacts (job_recruiter_contacts)
// past their life: 30 days, or 7 for an empty answer. The rows are third parties' work
// addresses, so this is retention, not housekeeping: it deletes by default and runs daily on a
// timer. A missed run only delays the deletion by a day; the API never serves an expired row.
//
// Needs DATABASE_URL.
package main

import (
	"context"
	"log"

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
	n, err := db.New(pool).DeleteExpiredJobRecruiterContacts(ctx)
	if err != nil {
		log.Printf("sweep-recruiter-contacts: %v", err)
		return 1
	}
	log.Printf("sweep-recruiter-contacts: deleted %d job(s)", n)
	return 0
}
