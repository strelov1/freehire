// Package testdb is the Postgres harness for the integration tests: one
// throwaway container per test binary, and a pristine database per test cloned
// from a template that carries every migration.
//
// The obvious alternative — a container per test — cost ~4.5s each (~3s to boot
// the container, ~1.5s to replay the 57 migrations) while the assertions
// themselves took milliseconds. At ~226 calls per CI run that was 8 of the
// backend job's 9.5 minutes. `CREATE DATABASE ... TEMPLATE` is a file copy of an
// already-migrated database, ~0.2s, and buys the same isolation: every test
// still gets its own database with its own sequences, so a test may freely
// mutate rows, indexes, or the schema.
//
// The container is reaped by the testcontainers ryuk sidecar when the test
// binary exits (nothing outlives the run unless TESTCONTAINERS_RYUK_DISABLED
// is set).
package testdb

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

const (
	// pgvector's build, not stock postgres: migrations run `CREATE EXTENSION
	// vector` (0092+), which the stock image doesn't ship. Must stay in sync with
	// docker-compose.yml's `db` service image — same Postgres major (18).
	image = "pgvector/pgvector:pg18"

	// templateDB holds the migrated schema that per-test databases are cloned
	// from. Nothing may hold a connection to it — CREATE DATABASE ... TEMPLATE
	// requires an idle source.
	templateDB = "hire_template"

	// adminDB is initdb's own database, used only to run CREATE/DROP DATABASE.
	adminDB = "postgres"

	user     = "hire"
	password = "hire"
)

type container struct {
	admin *pgxpool.Pool
	host  string
	port  string
}

var (
	once      sync.Once
	shared    *container
	bootErr   error
	dbCounter atomic.Uint64
)

// Pool hands the test its own migrated database. Both the pool and the database
// go away when the test ends.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	c := start(t)

	name := fmt.Sprintf("hire_test_%d", dbCounter.Add(1))
	if _, err := c.admin.Exec(ctx, `CREATE DATABASE "`+name+`" TEMPLATE `+templateDB); err != nil {
		t.Fatalf("clone %s: %v", templateDB, err)
	}

	// Cleanups are LIFO, so this drop runs after the pool below is closed. FORCE
	// evicts whatever connection a test left open, so one leak can't wedge the
	// rest of the suite.
	t.Cleanup(func() {
		_, _ = c.admin.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+name+`" WITH (FORCE)`)
	})

	pool, err := pgxpool.New(ctx, c.dsn(name))
	if err != nil {
		t.Fatalf("connect %s: %v", name, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func start(t *testing.T) *container {
	t.Helper()
	once.Do(func() { shared, bootErr = boot() })
	if bootErr != nil {
		t.Fatalf("start postgres: %v", bootErr)
	}
	return shared
}

func boot() (*container, error) {
	ctx := context.Background()

	scripts, err := migrationScripts()
	if err != nil {
		return nil, err
	}

	ctr, err := postgres.Run(ctx, image,
		postgres.WithDatabase(templateDB),
		postgres.WithUsername(user),
		postgres.WithPassword(password),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
		// The data is thrown away, so drop the durability the postgres module
		// doesn't already drop (it passes fsync=off itself).
		testcontainers.WithCmdArgs("-c", "full_page_writes=off", "-c", "synchronous_commit=off"),
	)
	if err != nil {
		return nil, fmt.Errorf("run container: %w", err)
	}

	host, err := ctr.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("container host: %w", err)
	}
	port, err := ctr.MappedPort(ctx, "5432/tcp")
	if err != nil {
		return nil, fmt.Errorf("container port: %w", err)
	}

	c := &container{host: host, port: port.Port()}
	admin, err := pgxpool.New(ctx, c.dsn(adminDB))
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", adminDB, err)
	}
	c.admin = admin
	return c, nil
}

func (c *container) dsn(database string) string {
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		user, password, c.host, c.port, database)
}

// migrationScripts lists migrations/*.sql in name order — the order Postgres
// initdb runs a mounted migrations/ dir in — so a new migration is never
// silently missing from the test schema.
func migrationScripts() ([]string, error) {
	root, err := repoRoot()
	if err != nil {
		return nil, err
	}
	scripts, err := filepath.Glob(filepath.Join(root, "migrations", "*.sql"))
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}
	if len(scripts) == 0 {
		return nil, fmt.Errorf("no migrations under %s", filepath.Join(root, "migrations"))
	}
	sort.Strings(scripts)
	return scripts, nil
}

// repoRoot walks up from the test's working directory to the module root, so the
// harness doesn't care how deep the package under test sits.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no go.mod above the working directory")
		}
		dir = parent
	}
}
