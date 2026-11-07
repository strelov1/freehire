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
// code a task brings. mise sets it for a laptop (scripts/dev-env.sh), and CI sets it
// for the integration shards, each of which starts one Postgres for all its binaries.
//
// On that server every test binary shares ONE template, named after a hash of the
// migrations, built by whichever binary gets there first and reused by the rest and
// by later runs. Migrating a template per binary cost ~2.4s each, 56 times a run,
// and left every one of them behind: nothing dropped a binary's own template when it
// exited. Postgres clones one template into many databases at once without
// complaint (checked with 12 concurrent CREATE DATABASE ... TEMPLATE), so the share
// needs a lock only around the build.
package testdb

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

	// sharedTemplatePrefix names the templates on an external server; the rest of the
	// name is the migrations' hash (templateName).
	sharedTemplatePrefix = "hire_tpl_"

	// templateLockKey serializes building a shared template, so concurrent test
	// binaries build it once. Listed with the project's other advisory-lock keys in
	// internal/platform/migrate.
	templateLockKey int64 = 0x66687470 // "fhtp"

	// staleTemplateAge is how long a template of OTHER migrations is kept: another
	// branch on the same server may still be using it, and rebuilding costs seconds.
	staleTemplateAge = 7 * 24 * time.Hour
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

// bootExternal points this binary at the shared template on a server that is already
// running, building it first when no binary has yet.
func bootExternal(ctx context.Context, adminURL string, scripts []string) (*container, error) {
	base, err := url.Parse(adminURL)
	if err != nil {
		return nil, fmt.Errorf("parse TESTDB_ADMIN_URL: %w", err)
	}
	name, err := templateName(scripts)
	if err != nil {
		return nil, err
	}
	c := &container{
		base:     *base,
		template: name,
		// The random part: a killed run leaves its databases behind on a server that
		// outlives it, and a later binary reusing the pid would collide with them.
		prefix: fmt.Sprintf("hire_test_%d_%s", os.Getpid(), strings.ToLower(rand.Text()[:6])),
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		return nil, fmt.Errorf("connect TESTDB_ADMIN_URL: %w", err)
	}
	if err := c.ensureTemplate(ctx, admin, scripts); err != nil {
		admin.Close()
		return nil, err
	}
	c.admin = admin
	return c, nil
}

// templateName is the shared template's name: a hash over every migration's file name
// and content, so a branch with other migrations gets a template of its own and an
// edited migration is never served from a template built before the edit.
func templateName(scripts []string) (string, error) {
	h := sha256.New()
	for _, s := range scripts {
		b, err := os.ReadFile(s)
		if err != nil {
			return "", fmt.Errorf("read migration: %w", err)
		}
		_, _ = fmt.Fprintf(h, "%s\x00%d\x00", filepath.Base(s), len(b)) // a hash never fails a write
		h.Write(b)
	}
	return sharedTemplatePrefix + hex.EncodeToString(h.Sum(nil))[:16], nil
}

// ensureTemplate builds the template unless it exists. The lock is a session lock on
// one connection held across the build, which runs in a psql process of its own; a
// binary that waited on it finds the template built and returns.
//
// The template is migrated under a temporary name and renamed into place, so the
// final name existing is what says the build finished: a run killed mid-build leaves
// only the temporary one, which the next builder drops.
func (c *container) ensureTemplate(ctx context.Context, admin *pgxpool.Pool, scripts []string) error {
	conn, err := admin.Acquire(ctx)
	if err != nil {
		return fmt.Errorf("acquire admin connection: %w", err)
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", templateLockKey); err != nil {
		return fmt.Errorf("lock template build: %w", err)
	}
	defer func() {
		_, _ = conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", templateLockKey)
	}()

	var exists bool
	if err := conn.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)", c.template).Scan(&exists); err != nil {
		return fmt.Errorf("look up %s: %w", c.template, err)
	}
	if exists {
		// Using it counts as building it, for dropStaleTemplates: another branch's build
		// takes this same lock, so a template in use is never a week old when it looks.
		return c.stamp(ctx, conn)
	}

	building := c.template + "_building"
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS "`+building+`" WITH (FORCE)`); err != nil {
		return fmt.Errorf("drop unfinished %s: %w", building, err)
	}
	if _, err := conn.Exec(ctx, `CREATE DATABASE "`+building+`"`); err != nil {
		return fmt.Errorf("create %s: %w", building, err)
	}
	if err := c.migrate(ctx, building, scripts); err != nil {
		_, _ = conn.Exec(context.Background(), `DROP DATABASE IF EXISTS "`+building+`" WITH (FORCE)`)
		return err
	}
	if _, err := conn.Exec(ctx, `ALTER DATABASE "`+building+`" RENAME TO "`+c.template+`"`); err != nil {
		return fmt.Errorf("finish %s: %w", c.template, err)
	}
	if err := c.stamp(ctx, conn); err != nil {
		return err
	}
	dropStaleTemplates(ctx, conn, c.template)
	return nil
}

// stamp records when the template was last built or used, for dropStaleTemplates:
// pg_database keeps no such time, so it goes in the database's comment.
func (c *container) stamp(ctx context.Context, conn *pgxpool.Conn) error {
	if _, err := conn.Exec(ctx, `COMMENT ON DATABASE "`+c.template+`" IS '`+time.Now().UTC().Format(time.RFC3339)+`'`); err != nil {
		return fmt.Errorf("stamp %s: %w", c.template, err)
	}
	return nil
}

// migrate applies the scripts through one psql session, file by file, as the image's
// initdb applies them: a migration marked no-transaction (CREATE INDEX CONCURRENTLY)
// cannot run as one multi-statement Exec, which Postgres wraps in an implicit
// transaction.
func (c *container) migrate(ctx context.Context, database string, scripts []string) error {
	// The password travels in PGPASSWORD, not in psql's argv, which any process list shows.
	target := c.base
	target.Path = "/" + database
	target.User = nil
	if name := c.base.User.Username(); name != "" {
		target.User = url.User(name)
	}
	args := []string{"-X", "-q", "-v", "ON_ERROR_STOP=1", "-d", target.String()}
	// RESET ALL between files: initdb gives each script a fresh session, and 0001 (a schema
	// dump) empties search_path for the rest of its session.
	for _, s := range scripts {
		args = append(args, "-c", "RESET ALL", "-f", s)
	}
	cmd := exec.CommandContext(ctx, "psql", args...)
	cmd.Env = os.Environ()
	if pw, ok := c.base.User.Password(); ok {
		cmd.Env = append(cmd.Env, "PGPASSWORD="+pw)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("migrate %s: %w\n%s", database, err, out)
	}
	return nil
}

// dropStaleTemplates drops templates of other migrations last built or used over
// staleTemplateAge ago. Best effort and never forced: a template another run is cloning right now is
// left for a later build to retry.
func dropStaleTemplates(ctx context.Context, conn *pgxpool.Conn, current string) {
	rows, err := conn.Query(ctx, `SELECT d.datname, coalesce(shobj_description(d.oid, 'pg_database'), '')
		FROM pg_database d WHERE d.datname LIKE $1 AND d.datname <> $2`, sharedTemplatePrefix+"%", current)
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var name, built string
		if rows.Scan(&name, &built) != nil {
			continue
		}
		if t, err := time.Parse(time.RFC3339, built); err == nil && time.Since(t) > staleTemplateAge {
			stale = append(stale, name)
		}
	}
	rows.Close()
	for _, name := range stale {
		_, _ = conn.Exec(ctx, `DROP DATABASE IF EXISTS "`+name+`"`)
	}
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
