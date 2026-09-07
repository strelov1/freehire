// Package migrate is the versioned migration runner for migrations/*.sql.
//
// Until now schema changes reached an existing database by hand: initdb applies
// the whole migrations/ dir once on first volume init, and every later file had
// to be run manually on prod before deploying. This package adds the missing
// bookkeeping: a schema_migrations table (version TEXT PK = the filename, so the
// historical number collisions like 0009×2 stay harmless), one transaction per
// migration, and a session advisory lock so two runs cannot interleave.
//
// Bootstrap of a pre-runner database is automatic: when schema_migrations is
// empty but the jobs table already exists (an initdb- or hand-migrated volume),
// the run baselines — records every on-disk migration as applied without
// executing it — instead of replaying files whose effects are already present.
// On a truly empty database every migration is applied in order.
package migrate

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// noTxMarker, placed as the WHOLE content of a comment line in a migration's leading
// comment block, opts the file out of the per-migration transaction. Needed for statements
// Postgres forbids inside a transaction block (CREATE INDEX CONCURRENTLY, DROP INDEX
// CONCURRENTLY).
//
// The file is then executed one statement at a time (see splitStatements) — a whole file in
// one Exec would still be one implicit transaction, which is not what the marker promises.
// The applied-record insert runs outside any transaction too, so a failure can leave the
// file executed but unrecorded — re-running is safe only if such files are written
// idempotently (IF NOT EXISTS / IF EXISTS).
const noTxMarker = "migrate: no-transaction"

// advisoryLockKey serializes concurrent migrate runs (e.g. a deploy racing a cron-triggered
// run). Arbitrary but stable — it only needs to not collide with the project's other
// advisory-lock users, which is the list a fourth one should read before picking its own:
//
//	728391      internal/platform/migrate     — this one, a blocking pg_advisory_lock
//	0x66686c76  cmd/liveness         — "fhlv", a non-blocking pg_try_advisory_lock
//	0x66686763  cmd/ghost-crosscheck — "fhgc", likewise
//	0x66687363  cmd/ingest-scheduler — "fhsc", likewise; it makes the fleet's concurrency
//	                                   cap atomic, which a check-then-act pair is not
//	0x66687277  cmd/billing-sync     — "fhrw", likewise; it serializes the referral grant
//	                                   pass, so the per-referrer reward ceiling is a bound
//	                                   rather than a suggestion. A count read in one
//	                                   statement and acted on in another is not atomic under
//	                                   READ COMMITTED, whichever statement holds it
//
// The comment here used to say "the project has none", which stopped being true when those two
// arrived and left the only place anyone would look asserting there was nothing to collide with.
const advisoryLockKey int64 = 728391

// lockTimeout bounds how long a migration may WAIT for a lock. It does not bound how long a
// statement may hold one, so a long index build is unaffected — only the wait is.
//
// Without it a schema change does not merely stall itself, it stalls the table. Postgres
// queues lock requests, so an `ALTER TABLE` waiting for ACCESS EXCLUSIVE sits ahead of every
// reader that arrives after it, and those readers wait too. That is how one bounded migration
// took out signed-in profile reads on 2026-07-30: a release landed inside the nightly
// pg_dump window, the dump holds ACCESS SHARE on every table for the length of the dump, and
// the ALTER queued behind it with live reads piling up behind the ALTER.
//
// Five seconds is far longer than an uncontended DDL lock takes and far shorter than anything
// worth stalling a table for. Failing is the right outcome: release.sh aborts the release with
// the live colour untouched, and the operator retries outside the window.
const lockTimeout = "5s"

// Migration is one migrations/*.sql file. Version is the filename — unique by
// construction, so the historical duplicate number prefixes (0009_job_daily_stats
// vs 0009_user_job_analysis) need no renumbering.
type Migration struct {
	Version string
	SQL     string
	// NoTx reports the file carries the noTxMarker and must run outside a
	// transaction.
	NoTx bool
}

// Load reads dir's *.sql files in lexicographic filename order — the same order
// Postgres initdb applies them on a fresh volume.
func Load(dir string) ([]Migration, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	migs := make([]Migration, 0, len(names))
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", name, err)
		}
		sql := string(data)
		migs = append(migs, Migration{Version: name, SQL: sql, NoTx: hasNoTxMarker(sql)})
	}
	return migs, nil
}

// hasNoTxMarker reports whether the file DECLARES the marker in its leading comment block
// (blank lines and -- comments before the first statement).
//
// The marker must be the whole content of its comment line. It is an instruction, not a
// topic: with strings.Contains, migrations/0126 — whose header says "No `migrate:
// no-transaction` marker, so the two statements are one transaction" — was opted out by the
// sentence explaining that it must not be. It survived only by accident, because a
// multi-statement simple query is one implicit transaction anyway.
//
// scripts/check-migrations.mjs repeats this rule; the two must agree, or squawk lints a
// file under a premise the runner does not share.
func hasNoTxMarker(sql string) bool {
	for _, line := range strings.Split(sql, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "--") {
			return false
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "--")) == noTxMarker {
			return true
		}
	}
	return false
}

// splitStatements cuts a migration into its top-level statements, on semicolons that are
// not inside a string, a dollar-quoted body, a quoted identifier, or a comment. Whitespace
// and comments are carried with the statement that follows them; a trailing fragment
// containing no statement is dropped.
//
// It exists because the no-transaction marker did not mean what its files said. The runner
// sent the whole file to one Exec, and pgx forces the simple protocol when a query carries
// no arguments (conn.go), which Postgres executes as ONE implicit transaction — so the
// marker cancelled only the runner's own BEGIN/COMMIT and bought nothing else.
// migrations/0135 states a guarantee it was not getting: "Two ALTER TABLE statements,
// deliberately NOT in one transaction: NOT VALID + VALIDATE CONSTRAINT in one transaction
// holds ACCESS EXCLUSIVE for the whole scan."
//
// This is a splitter, not a parser: it needs to find statement boundaries, not to
// understand SQL. The four quoting forms below are the only ones in which a semicolon can
// hide.
func splitStatements(sql string) []string {
	var (
		out   []string
		start int
	)
	for i := 0; i < len(sql); i++ {
		switch {
		case strings.HasPrefix(sql[i:], "--"):
			if end := strings.IndexByte(sql[i:], '\n'); end >= 0 {
				i += end
			} else {
				i = len(sql)
			}
		case strings.HasPrefix(sql[i:], "/*"):
			// Postgres nests block comments, so this counts depth rather than
			// stopping at the first close.
			depth, j := 1, i+2
			for j < len(sql) && depth > 0 {
				switch {
				case strings.HasPrefix(sql[j:], "/*"):
					depth++
					j += 2
				case strings.HasPrefix(sql[j:], "*/"):
					depth--
					j += 2
				default:
					j++
				}
			}
			i = j - 1
		case sql[i] == '\'':
			i = closeQuoted(sql, i, '\'')
		case sql[i] == '"':
			i = closeQuoted(sql, i, '"')
		case sql[i] == '$':
			if tag, after, ok := dollarTag(sql, i); ok {
				if end := strings.Index(sql[after:], tag); end >= 0 {
					i = after + end + len(tag) - 1
				} else {
					i = len(sql)
				}
			}
		case sql[i] == ';':
			if s := sql[start:i]; strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
			start = i + 1
		}
	}
	if tail := sql[min(start, len(sql)):]; hasStatement(tail) {
		out = append(out, tail)
	}
	return out
}

// closeQuoted returns the index of the closing quote for the literal or identifier opening
// at i. A doubled quote is an escaped one and does not close.
func closeQuoted(sql string, i int, quote byte) int {
	for j := i + 1; j < len(sql); j++ {
		if sql[j] != quote {
			continue
		}
		if j+1 < len(sql) && sql[j+1] == quote {
			j++
			continue
		}
		return j
	}
	return len(sql)
}

// dollarTag reads a dollar-quote opener at i ($$ or $tag$), returning the tag to look for
// and the index just past the opener. A '$' that opens no tag (a positional parameter, or
// a '$' inside an identifier) reports ok=false.
func dollarTag(sql string, i int) (tag string, after int, ok bool) {
	for j := i + 1; j < len(sql); j++ {
		c := sql[j]
		if c == '$' {
			return sql[i : j+1], j + 1, true
		}
		if !tagByte(c, j == i+1) {
			return "", 0, false
		}
	}
	return "", 0, false
}

// tagByte reports whether c may appear in a dollar-quote tag. A tag may not START with a
// digit, which is what separates $1 (a positional parameter) from $t1$ (a tag).
func tagByte(c byte, first bool) bool {
	if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
		return true
	}
	return !first && c >= '0' && c <= '9'
}

// hasStatement reports whether a trailing fragment carries anything but whitespace and
// comments — a file's closing commentary is not a statement to execute.
func hasStatement(fragment string) bool {
	for _, line := range strings.Split(fragment, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "--") {
			return true
		}
	}
	return false
}

// plan is one run's work: baseline records versions without executing them,
// apply executes then records.
type plan struct {
	baseline []Migration
	apply    []Migration
}

// decide routes every unapplied migration to baseline or apply. legacySchema
// (schema_migrations empty but the jobs table present) means the database
// predates the runner, so on-disk files are already in effect and must only be
// recorded; forceBaseline is the operator's explicit -baseline flag.
func decide(migs []Migration, applied map[string]bool, legacySchema, forceBaseline bool) plan {
	var p plan
	for _, m := range migs {
		if applied[m.Version] {
			continue
		}
		if legacySchema || forceBaseline {
			p.baseline = append(p.baseline, m)
		} else {
			p.apply = append(p.apply, m)
		}
	}
	return p
}

// Runner applies migrations against a Postgres pool.
type Runner struct {
	pool *pgxpool.Pool
}

// NewRunner returns a Runner applying migrations through pool.
func NewRunner(pool *pgxpool.Pool) *Runner {
	return &Runner{pool: pool}
}

// Run brings the database up to date with migs. forceBaseline records every
// unapplied migration without executing it (operator bootstrap of a database
// whose schema is already current). It returns the versions baselined and
// applied, in order.
func (r *Runner) Run(ctx context.Context, migs []Migration, forceBaseline bool) (baselined, applied []string, err error) {
	conn, err := r.pool.Acquire(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("acquire connection: %w", err)
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockKey); err != nil {
		return nil, nil, fmt.Errorf("advisory lock: %w", err)
	}
	defer func() {
		// Best-effort release; the lock is session-scoped and dies with the
		// connection anyway.
		if _, err := conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockKey); err != nil {
			log.Printf("migrate: advisory unlock: %v", err)
		}
	}()

	// Set after the advisory lock deliberately: a second concurrent run should still WAIT its
	// turn for that (it is not a table lock and blocks nobody), while every table lock taken
	// below is bounded. Session-level, so it covers the no-transaction files too.
	if _, err := conn.Exec(ctx, "SET lock_timeout = '"+lockTimeout+"'"); err != nil {
		return nil, nil, fmt.Errorf("set lock_timeout: %w", err)
	}

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
    version text PRIMARY KEY,
    applied_at timestamptz NOT NULL DEFAULT now()
)`); err != nil {
		return nil, nil, fmt.Errorf("ensure schema_migrations: %w", err)
	}

	appliedSet, err := appliedVersions(ctx, conn)
	if err != nil {
		return nil, nil, err
	}

	legacy := false
	if len(appliedSet) == 0 {
		legacy, err = hasJobsTable(ctx, conn)
		if err != nil {
			return nil, nil, err
		}
	}

	p := decide(migs, appliedSet, legacy, forceBaseline)
	for v := range appliedSet {
		if !onDisk(migs, v) {
			log.Printf("migrate: %s recorded as applied but missing on disk", v)
		}
	}

	if len(p.baseline) > 0 {
		if err := recordBaseline(ctx, conn, p.baseline); err != nil {
			return nil, nil, err
		}
		for _, m := range p.baseline {
			baselined = append(baselined, m.Version)
		}
		if legacy && !forceBaseline {
			log.Printf("migrate: existing schema without schema_migrations — baselined %d migration(s)", len(p.baseline))
		}
	}

	for _, m := range p.apply {
		if err := applyOne(ctx, conn, m); err != nil {
			return baselined, applied, err
		}
		applied = append(applied, m.Version)
		log.Printf("migrate: applied %s", m.Version)
	}
	return baselined, applied, nil
}

// applyOne executes one migration and records it, atomically unless the file
// opted out of transactions.
func applyOne(ctx context.Context, conn *pgxpool.Conn, m Migration) error {
	if m.NoTx {
		return applyNoTx(ctx, conn, m)
	}

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin %s: %w", m.Version, err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, m.SQL); err != nil {
		return fmt.Errorf("apply %s: %w", m.Version, err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO public.schema_migrations (version) VALUES ($1)", m.Version); err != nil {
		return fmt.Errorf("record %s: %w", m.Version, err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit %s: %w", m.Version, err)
	}
	return nil
}

// applyNoTx executes a no-transaction migration one statement at a time and records it.
//
// One statement per Exec is what makes the marker mean what its files say: a whole file in
// one Exec goes out on the simple protocol (pgx forces it when there are no arguments) and
// Postgres runs a multi-statement simple query as ONE implicit transaction, so the marker
// cancelled only this runner's own BEGIN/COMMIT. Each statement still gets an implicit
// transaction of its own, which is the point — that is what a CONCURRENTLY statement needs
// and what keeps 0135's NOT VALID and VALIDATE CONSTRAINT from sharing an ACCESS EXCLUSIVE
// lock across the whole scan.
//
// A failure part-way leaves the file executed-but-unrecorded, which is why these files must
// be written idempotently (IF NOT EXISTS / IF EXISTS) — the same contract as before, now
// with a finer failure granularity.
func applyNoTx(ctx context.Context, conn *pgxpool.Conn, m Migration) error {
	for _, stmt := range splitStatements(m.SQL) {
		if _, err := conn.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("apply %s: %w", m.Version, err)
		}
	}

	// A successful CREATE INDEX CONCURRENTLY is not proof of a usable index. It waits on
	// virtual transaction locks and old snapshots with an ordinary wait, which lockTimeout
	// aborts with 55P03, leaving the index at indisvalid=f. The FIRST run fails loudly and
	// records nothing. The SECOND is the trap: `IF NOT EXISTS` sees the carcass, does
	// nothing, succeeds, and the version is recorded against an index that will never be
	// used. migrations/0126 and 0127 record that happening on 2026-09-03.
	//
	// So the check cannot be before-and-after — on the re-run the carcass is present in
	// BOTH snapshots. It asks instead whether any index THIS FILE NAMES is invalid, which
	// is also what keeps somebody else's older carcass from failing every run forever.
	broke, err := invalidIndexesNamedIn(ctx, conn, m.SQL)
	if err != nil {
		return fmt.Errorf("read index validity after %s: %w", m.Version, err)
	}
	if len(broke) > 0 {
		return fmt.Errorf("apply %s: %s left invalid (indisvalid=f) — DROP INDEX and re-run; the version was NOT recorded",
			m.Version, strings.Join(broke, ", "))
	}

	if _, err := conn.Exec(ctx, "INSERT INTO public.schema_migrations (version) VALUES ($1)", m.Version); err != nil {
		return fmt.Errorf("record %s: %w", m.Version, err)
	}
	return nil
}

// invalidIndexesNamedIn returns the invalid indexes whose name appears as a word in sql,
// sorted. Matching on the file's own text is coarse but exact where it matters: a
// CONCURRENTLY file names the index it builds, and no other file's carcass is named here.
func invalidIndexesNamedIn(ctx context.Context, conn *pgxpool.Conn, sql string) ([]string, error) {
	rows, err := conn.Query(ctx,
		`SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid = i.indexrelid WHERE NOT i.indisvalid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	lower := strings.ToLower(sql)
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if namesIdentifier(lower, strings.ToLower(name)) {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// namesIdentifier reports whether sql mentions name as a whole identifier, so a short index
// name is not matched inside a longer one.
func namesIdentifier(sql, name string) bool {
	for i := 0; ; {
		j := strings.Index(sql[i:], name)
		if j < 0 {
			return false
		}
		j += i
		if !identifierByte(byteAt(sql, j-1)) && !identifierByte(byteAt(sql, j+len(name))) {
			return true
		}
		i = j + 1
	}
}

func byteAt(s string, i int) byte {
	if i < 0 || i >= len(s) {
		return ' '
	}
	return s[i]
}

func identifierByte(c byte) bool {
	return c == '_' || c == '$' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// appliedVersions reads the recorded versions.
func appliedVersions(ctx context.Context, conn *pgxpool.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, "SELECT version FROM public.schema_migrations")
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	versions, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	set := make(map[string]bool, len(versions))
	for _, v := range versions {
		set[v] = true
	}
	return set, nil
}

// hasJobsTable reports whether the core schema already exists — the legacy
// signal that the database predates the runner and must be baselined, not
// replayed.
func hasJobsTable(ctx context.Context, conn *pgxpool.Conn) (bool, error) {
	var regclass *string
	if err := conn.QueryRow(ctx, "SELECT to_regclass('public.jobs')::text").Scan(&regclass); err != nil {
		return false, fmt.Errorf("probe jobs table: %w", err)
	}
	return regclass != nil, nil
}

// recordBaseline inserts versions in one transaction.
func recordBaseline(ctx context.Context, conn *pgxpool.Conn, migs []Migration) error {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin baseline: %w", err)
	}
	defer tx.Rollback(ctx)

	for _, m := range migs {
		if _, err := tx.Exec(ctx, "INSERT INTO public.schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING", m.Version); err != nil {
			return fmt.Errorf("baseline %s: %w", m.Version, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit baseline: %w", err)
	}
	return nil
}

// onDisk reports whether version is among the loaded migrations.
func onDisk(migs []Migration, version string) bool {
	for _, m := range migs {
		if m.Version == version {
			return true
		}
	}
	return false
}
