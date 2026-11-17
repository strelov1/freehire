package job_test

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Identity resolution and both writes take a transaction BY TYPE, so identity locks
// survive the lookup and the full write and observation cannot use separate commits.
var (
	_ func(context.Context, pgx.Tx, db.UpsertJobParams) (db.UpsertJobRow, error)    = job.Upsert
	_ func(context.Context, pgx.Tx, db.UpsertJobParams) (db.UpsertJobParams, error) = job.ResolveUpsertIdentity
	_ func(context.Context, pgx.Tx, int64, bool, bool) error                        = job.Observe
)
