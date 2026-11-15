//go:build integration

package migrate_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// legacyActivityPool preserves the original release A regression scenarios even
// when the shared template has already applied the contract migration.
func legacyActivityPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool := bootstrapped(t)
	testdb.InstallLegacyJobActivity(t, pool)
	return pool
}
