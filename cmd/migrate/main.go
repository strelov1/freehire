// Command migrate is the versioned migration runner for migrations/*.sql. It is
// a run-once-and-exit worker: record every new migration file in
// schema_migrations after executing it, in lexicographic filename order, one
// transaction per file, under a session advisory lock.
//
//	migrate [-dir migrations] [-baseline]   # needs DATABASE_URL
//
// Run it before deploying code that reads new schema. A database that predates the runner
// (schema present, schema_migrations empty) is baselined automatically: every on-disk file is
// recorded as applied without re-executing it. -baseline forces that recording.
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/platform/migrate"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	dir := flag.String("dir", "migrations", "directory of *.sql migration files")
	baseline := flag.Bool("baseline", false, "record all unapplied migrations as applied without executing them")
	flag.Parse()

	migs, err := migrate.Load(*dir)
	if err != nil {
		log.Printf("load migrations: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	baselined, applied, err := migrate.NewRunner(pool).Run(ctx, migs, *baseline)
	if err != nil {
		log.Printf("migrate: %v", err)
		return 1
	}

	log.Printf("migrate: %d file(s) on disk, %d baselined, %d applied", len(migs), len(baselined), len(applied))
	return 0
}
