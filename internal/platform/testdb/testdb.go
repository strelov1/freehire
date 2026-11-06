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
//
// TESTDB_ADMIN_URL points the harness at a Postgres that is already running
// instead (pgvector installed, a role that may CREATE DATABASE, and `psql` on
// PATH). It exists for the coding agents' runner, which has no Docker and must
// not get the host's: a Docker socket there would be root on the host for any
// code a task brings. Each test binary migrates its own template on that
// server, named after its pid, because `go test ./...` runs binaries in
// parallel and a template being cloned must have no other connection.
package testdb

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/modroot"
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
	// base is the server's address and credentials; dsn swaps in the database.
	base url.URL
	// template is the migrated database each test clones.
	template string
	// prefix keeps this binary's databases apart from another's on a shared server.
	prefix string
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

	name := fmt.Sprintf("%s_%d", c.prefix, dbCounter.Add(1))
	if _, err := c.admin.Exec(ctx, `CREATE DATABASE "`+name+`" TEMPLATE "`+c.template+`"`); err != nil {
		t.Fatalf("clone %s: %v", c.template, err)
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
	if admin := os.Getenv("TESTDB_ADMIN_URL"); admin != "" {
		return bootExternal(ctx, admin, scripts)
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

	c := &container{
		base: url.URL{
			Scheme: "postgres", User: url.UserPassword(user, password),
			Host: host + ":" + port.Port(), RawQuery: "sslmode=disable",
		},
		template: templateDB,
		prefix:   "hire_test",
	}
	admin, err := pgxpool.New(ctx, c.dsn(adminDB))
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", adminDB, err)
	}
	c.admin = admin
	return c, nil
}

// bootExternal migrates this binary's own template on a server that is already
// running. The scripts go through one psql session, file by file, as the image's
// initdb applies them: a migration marked no-transaction (CREATE INDEX
// CONCURRENTLY) cannot run as one multi-statement Exec, which Postgres wraps in
// an implicit transaction.
func bootExternal(ctx context.Context, adminURL string, scripts []string) (*container, error) {
	base, err := url.Parse(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse TESTDB_ADMIN_URL: %w", err)
	}
	c := &container{
		base:     *base,
		template: fmt.Sprintf("hire_template_%d", os.Getpid()),
		prefix:   fmt.Sprintf("hire_test_%d", os.Getpid()),
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return nil, fmt.Errorf("connect TESTDB_ADMIN_URL: %w", err)
	}
	// A pid recycled from an earlier run on the same server may have left one behind.
	if _, err := admin.Exec(ctx, `DROP DATABASE IF EXISTS "`+c.template+`" WITH (FORCE)`); err != nil {
		admin.Close()
		return nil, fmt.Errorf("drop stale %s: %w", c.template, err)
	}
	if _, err := admin.Exec(ctx, `CREATE DATABASE "`+c.template+`"`); err != nil {
		admin.Close()
		return nil, fmt.Errorf("create %s: %w", c.template, err)
	}
	args := []string{"-X", "-q", "-v", "ON_ERROR_STOP=1", "-d", c.dsn(c.template)}
	// RESET ALL between files: initdb gives each script a fresh session, and 0001 (a schema
	// dump) empties search_path for the rest of its session.
	for _, s := range scripts {
		args = append(args, "-c", "RESET ALL", "-f", s)
	}
	if out, err := exec.CommandContext(ctx, "psql", args...).CombinedOutput(); err != nil {
		admin.Close()
		return nil, fmt.Errorf("migrate %s: %w\n%s", c.template, err, out)
	}
	c.admin = admin
	return c, nil
}

func (c *container) dsn(database string) string {
	u := c.base
	u.Path = "/" + database
	return u.String()
}

// migrationScripts lists migrations/*.sql in name order — the order Postgres
// initdb runs a mounted migrations/ dir in — so a new migration is never
// silently missing from the test schema.
func migrationScripts() ([]string, error) {
	root, err := modroot.Find()
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
