// Command report-cv-letters is a hand-run, read-only report on the CV lifecycle letters: per
// letter, how many were sent, delivered and clicked and how many recipients started paying
// within 14 days, against the 10% holdout that was never mailed. It writes nothing and is
// not on a timer. Needs only DATABASE_URL.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/engage/cvlifecycle"
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

	report, err := cvlifecycle.BuildReport(ctx, db.New(pool), cvlifecycle.DefaultConfig())
	if err != nil {
		log.Printf("report-cv-letters: %v", err)
		return 1
	}
	if err := report.Write(os.Stdout); err != nil {
		log.Printf("report-cv-letters: %v", err)
		return 1
	}
	return 0
}
