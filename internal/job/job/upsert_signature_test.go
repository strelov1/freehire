package job_test

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Both writes take a transaction BY TYPE, so a caller cannot run the full write and its
// observation as two autocommit statements.
var (
	_ func(context.Context, pgx.Tx, db.UpsertJobParams) (db.UpsertJobRow, error) = job.Upsert
	_ func(context.Context, pgx.Tx, int64, bool, bool) error                     = job.Observe
)
