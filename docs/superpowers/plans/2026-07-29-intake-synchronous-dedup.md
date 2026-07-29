# Synchronous dedup at intake — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stop a storefront link from writing a second catalogue row for a vacancy we already
carry, whatever ATS sits behind it.

**Architecture:** Before `linkimport.write` upserts a `weblink` row, it asks the catalogue for an
open canonical posting in the same role cluster (`company_slug` + `role_fingerprint`). On a hit
the row is still written — it is what makes the storefront URL resolvable — but marked
`duplicate_of` the canon and skipped for enrichment. The intake then answers `found` with the
canonical slug, after recording the contribution.

**Tech Stack:** Go, pgx, sqlc, Postgres. Tests are Go integration tests behind the
`integration` build tag (testcontainers-backed Postgres).

**Spec:** `docs/superpowers/specs/2026-07-29-intake-synchronous-dedup-design.md`

## Global Constraints

- English only in code, comments, identifiers, commits.
- `internal/db/` is generated. Edit `internal/db/queries/*.sql`, then run `make sqlc`. Never
  hand-edit `internal/db/*.sql.go`.
- Never edit an applied migration. This plan needs no migration — the
  `(company_slug, role_fingerprint)` index already exists (`migrations/0003_role_fingerprint.sql`).
- Integration tests run with `go test -tags=integration ./internal/<pkg>/` and need Docker.
- Branch: `feat/intake-sync-dedup` (already created off `origin/main`, carries the spec commit).

---

### Task 1: The two queries

**Files:**
- Modify: `internal/db/queries/jobs.sql` (add two queries)
- Generated (do not hand-edit): `internal/db/jobs.sql.go`, `internal/db/querier.go`
- Test: `internal/db/canonical_job_for_role_integration_test.go` (create)

**Interfaces:**
- Produces: `db.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{CompanySlug, RoleFingerprint, Source, ExternalID string}) (db.CanonicalJobForRoleRow, error)` returning `{ID int64; PublicSlug string}`, `pgx.ErrNoRows` when there is no canon.
- Produces: `db.MarkJobDuplicateOf(ctx, db.MarkJobDuplicateOfParams{ID, DuplicateOf int64}) (int64, error)` — rows affected.

- [ ] **Step 1: Write the failing test**

Create `internal/db/canonical_job_for_role_integration_test.go`:

```go
//go:build integration

// Integration tests for the intake's synchronous dedup lookup: which posting, if any, a
// freshly imported storefront row should be marked a duplicate of.
// Run with: go test -tags=integration ./internal/db/
//
// NOTE: package db, not db_test — the existing integration tests here live inside the package
// and share the startPostgres helper from enrichment_integration_test.go. That is why the types
// below carry no db. prefix.
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestCanonicalJobForRole(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	seed := func(t *testing.T, source, externalID, slug, fp string, closed, dup bool) int64 {
		t.Helper()
		var id int64
		err := pool.QueryRow(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug,
			                  role_fingerprint, closed_at)
			VALUES ($1, $2, 'https://x.test/'||$2, 'Senior Go Engineer', $3, 'Acme', 'acme', $4,
			        CASE WHEN $5 THEN now() ELSE NULL END)
			RETURNING id`, source, externalID, slug, fp, closed).Scan(&id)
		if err != nil {
			t.Fatalf("seed %s: %v", externalID, err)
		}
		if dup {
			if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of = $1 WHERE id = $2`, id-1, id); err != nil {
				t.Fatalf("mark %s duplicate: %v", externalID, err)
			}
		}
		return id
	}

	canonID := seed(t, "greenhouse", "acme:1", "senior-go-acme-1", "fp-go", false, false)

	t.Run("finds the open canonical posting of the role", func(t *testing.T) {
		row, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: "fp-go",
			Source: "weblink", ExternalID: "https://storefront.test/go",
		})
		if err != nil {
			t.Fatalf("CanonicalJobForRole: %v", err)
		}
		if row.ID != canonID || row.PublicSlug != "senior-go-acme-1" {
			t.Errorf("= (%d, %q), want (%d, senior-go-acme-1)", row.ID, row.PublicSlug, canonID)
		}
	})

	t.Run("ignores the row being imported", func(t *testing.T) {
		// A re-import of the same URL must not find itself and mark itself its own duplicate.
		seed(t, "weblink", "https://storefront.test/go", "senior-go-acme-2", "fp-go", false, false)
		row, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: "fp-go",
			Source: "weblink", ExternalID: "https://storefront.test/go",
		})
		if err != nil {
			t.Fatalf("CanonicalJobForRole: %v", err)
		}
		if row.ID != canonID {
			t.Errorf("canon = %d, want %d — the imported row must be excluded", row.ID, canonID)
		}
	})

	t.Run("no canon when every candidate is closed or itself a duplicate", func(t *testing.T) {
		seed(t, "greenhouse", "acme:9", "closed-role-acme", "fp-closed", true, false)
		_, err := q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
			CompanySlug: "acme", RoleFingerprint: "fp-closed",
			Source: "weblink", ExternalID: "https://storefront.test/closed",
		})
		if !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("err = %v, want pgx.ErrNoRows — a closed posting is no canon", err)
		}
	})
}

func TestMarkJobDuplicateOf(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	var canonID, dupID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		VALUES ('greenhouse', 'acme:1', 'https://x.test/1', 'Senior Go', 'canon-slug', 'acme')
		RETURNING id`).Scan(&canonID); err != nil {
		t.Fatalf("seed canon: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company_slug)
		VALUES ('weblink', 'https://x.test/store', 'https://x.test/store', 'Senior Go', 'dup-slug', 'acme')
		RETURNING id`).Scan(&dupID); err != nil {
		t.Fatalf("seed duplicate: %v", err)
	}

	n, err := q.MarkJobDuplicateOf(ctx, MarkJobDuplicateOfParams{ID: dupID, DuplicateOf: canonID})
	if err != nil || n != 1 {
		t.Fatalf("MarkJobDuplicateOf = (%d, %v), want (1, nil)", n, err)
	}
	var got int64
	if err := pool.QueryRow(ctx, `SELECT duplicate_of FROM jobs WHERE id = $1`, dupID).Scan(&got); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if got != canonID {
		t.Errorf("duplicate_of = %d, want %d", got, canonID)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags=integration ./internal/db/ -run 'TestCanonicalJobForRole|TestMarkJobDuplicateOf'`
Expected: compile failure — `q.CanonicalJobForRole` and `q.MarkJobDuplicateOf` are undefined.

- [ ] **Step 3: Add the queries**

Append to `internal/db/queries/jobs.sql`, right after `RecomputeRoleDuplicatesForCompany`:

```sql
-- name: CanonicalJobForRole :one
-- The open canonical posting of one role cluster, asked by the intake BEFORE it writes an
-- imported storefront row: a careers site on a company's own domain fronts an ATS board, and
-- without this the same vacancy lands twice — once from the crawl, once from the link.
-- Deliberately mirrors RecomputeRoleDuplicatesForCompany's choice (MIN(id) among the open rows
-- of the cluster) so this answer and the one the reindex computes later agree.
-- A canon must be open and not itself a duplicate, or marking would build a chain no reader
-- expects. The row being imported is excluded by its own dedup identity, because a re-import
-- of the same URL would otherwise find itself. Served by jobs_company_slug_idx.
SELECT id, public_slug
FROM jobs
WHERE company_slug = sqlc.arg(company_slug)
  AND role_fingerprint = sqlc.arg(role_fingerprint)
  AND closed_at IS NULL
  AND duplicate_of IS NULL
  AND NOT (source = sqlc.arg(source) AND external_id = sqlc.arg(external_id))
ORDER BY id
LIMIT 1;

-- name: MarkJobDuplicateOf :execrows
-- Point one row at its canon. Used by the import path only: the batch dedup passes recompute
-- whole companies (RecomputeRoleDuplicatesForCompany / SuppressAggregatorDuplicatesForCompany)
-- and must keep doing so — this marks the single row an import just wrote.
UPDATE jobs
SET duplicate_of = sqlc.arg(duplicate_of),
    updated_at   = now()
WHERE id = sqlc.arg(id);
```

- [ ] **Step 4: Regenerate and run the test**

Run: `make sqlc && go test -tags=integration ./internal/db/ -run 'TestCanonicalJobForRole|TestMarkJobDuplicateOf'`
Expected: PASS.

Check the generated signature is the one later tasks rely on:
`grep -n "func (q \*Queries) CanonicalJobForRole" internal/db/jobs.sql.go`

- [ ] **Step 5: Commit**

```bash
git add internal/db/queries/jobs.sql internal/db/jobs.sql.go internal/db/querier.go \
        internal/db/canonical_job_for_role_integration_test.go
git commit -m "feat(db): ask for the canonical posting of a role cluster"
```

---

### Task 2: Mark the imported row

**Files:**
- Modify: `internal/linkimport/linkimport.go` (`Result` struct ~line 33; `write` ~line 170)
- Test: `internal/linkimport/import_integration_test.go`

**Interfaces:**
- Consumes: `db.CanonicalJobForRole`, `db.MarkJobDuplicateOf` from Task 1.
- Produces: `linkimport.Result` gains `Deduped bool`. On a hit, `Result.PublicSlug` is the
  CANONICAL row's slug, not the written row's.

- [ ] **Step 1: Write the failing test**

Append to `internal/linkimport/import_integration_test.go`:

```go
func TestImportCollapsesOntoACrawledPosting(t *testing.T) {
	// A storefront over an ATS board we already crawl. The page parses fine, so the generic
	// resolver would file it under (weblink, <the URL>) — a second row for a vacancy the
	// catalogue already holds. It must be written, but marked a duplicate of the crawled row,
	// and left out of the enrichment queue.
	pool := startPostgres(t)
	ctx := context.Background()
	q := New(pool)

	var canonID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
		VALUES ('greenhouse', 'mindera:1', 'https://boards.greenhouse.io/mindera/jobs/1',
		        'Staff Java Backend Developer', 'staff-java-mindera', 'Mindera', 'mindera', $1)
		RETURNING id`, fingerprintOf(t, "Staff Java Backend Developer", "mindera")).Scan(&canonID); err != nil {
		t.Fatalf("seed the crawled posting: %v", err)
	}

	const pageURL = "https://careers.mindera.test/jobs/staff-java-backend-developer"
	im := linkimport.New(pool, q, nil, pagesClient{"/jobs/staff-java-backend-developer": vacancyPage}, nil, nil)

	res, ok, err := im.Import(ctx, pageURL, linkimport.Board{})
	if err != nil || !ok {
		t.Fatalf("Import = (ok %v, err %v), want the vacancy imported", ok, err)
	}
	if !res.Deduped {
		t.Error("Deduped = false, want true — the catalogue already carries this vacancy")
	}
	if res.PublicSlug != "staff-java-mindera" {
		t.Errorf("PublicSlug = %q, want the canonical slug staff-java-mindera", res.PublicSlug)
	}

	var dupOf *int64
	if err := pool.QueryRow(ctx,
		`SELECT duplicate_of FROM jobs WHERE source = 'weblink' AND external_id = $1`, pageURL).Scan(&dupOf); err != nil {
		t.Fatalf("read the written row: %v", err)
	}
	if dupOf == nil || *dupOf != canonID {
		t.Errorf("duplicate_of = %v, want %d", dupOf, canonID)
	}

	var queued int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM enrichment_outbox o
		JOIN jobs j ON j.id = o.job_id
		WHERE j.source = 'weblink'`).Scan(&queued); err != nil {
		t.Fatalf("count enrichment queue: %v", err)
	}
	if queued != 0 {
		t.Errorf("enrichment queue holds %d rows for the duplicate, want 0 — it never reaches search", queued)
	}
}
```

Add the helper the test uses, in the same file:

```go
// fingerprintOf builds the role fingerprint the import will derive for a title, so a seeded
// row lands in the same role cluster. Mirrors what write() does via jobhash.RoleFingerprint.
func fingerprintOf(t *testing.T, title, companySlug string) string {
	t.Helper()
	return jobhash.RoleFingerprint(db.UpsertJobParams{Title: title, CompanySlug: companySlug})
}
```

Imports to add to the test file: `"github.com/strelov1/freehire/internal/jobhash"`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags=integration ./internal/linkimport/ -run TestImportCollapsesOntoACrawledPosting`
Expected: compile failure — `res.Deduped` undefined.

- [ ] **Step 3: Add the field**

In `internal/linkimport/linkimport.go`, extend `Result`:

```go
	// Deduped reports that the catalogue already held this vacancy under a crawled source, so
	// the row just written was marked a duplicate of it and PublicSlug names the CANONICAL
	// posting rather than the row this import wrote. The row is written rather than skipped
	// because it is what makes the storefront URL resolvable at all — FindOpenJobByURL matches
	// duplicates and answers with the posting they duplicate.
	Deduped bool
```

- [ ] **Step 4: Run the test to see it fail on behaviour, not compilation**

Run: `go test -tags=integration ./internal/linkimport/ -run TestImportCollapsesOntoACrawledPosting`
Expected: FAIL with `Deduped = false, want true`.

- [ ] **Step 5: Implement the dedup in write**

In `internal/linkimport/linkimport.go`, replace the body of `write` between `params.RoleFingerprint = …`
and `return Result{…}` with:

```go
	// A page read by the generic resolver is filed under its own URL, so nothing stops it from
	// becoming a second row for a vacancy a crawl already wrote. Ask before writing. Only the
	// generic identity needs this: every board identity is deduped by UpsertJob's
	// ON CONFLICT (source, external_id).
	canon, deduped := im.canonicalForRole(ctx, params)

	tx, err := im.pool.Begin(ctx)
	if err != nil {
		return Result{}, false, err
	}
	defer tx.Rollback(ctx)
	qtx := im.q.WithTx(tx)
	res, err := qtx.UpsertJob(ctx, params)
	if err != nil {
		return Result{}, false, err
	}
	if deduped {
		if _, err := qtx.MarkJobDuplicateOf(ctx, db.MarkJobDuplicateOfParams{
			ID: res.Job.ID, DuplicateOf: canon.ID,
		}); err != nil {
			return Result{}, false, err
		}
	} else if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		// A duplicate never reaches search, so enriching it pays an LLM for an invisible row.
		TargetVersion:     int32(enrich.Version),
		JobID:             res.Job.ID,
		ExcludeCategories: vocab.NonTechCategories,
	}); err != nil {
		return Result{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, false, err
	}
	if !deduped {
		im.index(ctx, res)
	}

	out := Result{
		Source:      res.Job.Source,
		ExternalID:  res.Job.ExternalID,
		PublicSlug:  res.Job.PublicSlug,
		CompanySlug: res.Job.CompanySlug,
		Deduped:     deduped,
	}
	if deduped {
		out.PublicSlug = canon.PublicSlug
	}
	return out, true, nil
}

// canonicalForRole asks whether the catalogue already holds this vacancy in an open, canonical
// row — the check that keeps a storefront link from duplicating the posting it fronts. Only
// asked for the generic identity, which is the one keyed by a URL rather than by a board.
//
// A lookup failure is not fatal: the row is written unmarked, exactly as before. Dedup is an
// improvement, not a condition of keeping the vacancy.
func (im *Importer) canonicalForRole(ctx context.Context, p db.UpsertJobParams) (db.CanonicalJobForRoleRow, bool) {
	if p.Source != linksource.GenericSource || !p.RoleFingerprint.Valid || p.RoleFingerprint.String == "" {
		return db.CanonicalJobForRoleRow{}, false
	}
	row, err := im.q.CanonicalJobForRole(ctx, CanonicalJobForRoleParams{
		CompanySlug:     p.CompanySlug,
		RoleFingerprint: p.RoleFingerprint.String,
		Source:          p.Source,
		ExternalID:      p.ExternalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.CanonicalJobForRoleRow{}, false
	}
	if err != nil {
		log.Printf("linkimport: canonical posting for %s/%s: %v", p.CompanySlug, p.RoleFingerprint.String, err)
		return db.CanonicalJobForRoleRow{}, false
	}
	return row, true
}
```

Add imports if missing: `"errors"` and `"github.com/jackc/pgx/v5"`.

- [ ] **Step 6: Run the whole package**

Run: `go test -tags=integration ./internal/linkimport/`
Expected: PASS, including the pre-existing tests (an ordinary import with no canon must still
be enqueued and indexed).

- [ ] **Step 7: Prove the test catches the regression**

Temporarily change `canonicalForRole` to `return db.CanonicalJobForRoleRow{}, false` as its
first statement, run the test, confirm it FAILS, then restore.

- [ ] **Step 8: Commit**

```bash
git add internal/linkimport/
git commit -m "feat(linkimport): a storefront import collapses onto the posting it duplicates"
```

---

### Task 3: The intake answers found

**Files:**
- Modify: `internal/handler/intake.go` (`Resolve`)
- Test: `internal/handler/resolve_job_integration_test.go`

**Interfaces:**
- Consumes: `linkimport.Result.Deduped` from Task 2.
- Produces: no new wire status. `found` becomes reachable a second way — after the import,
  with the contribution recorded first.

- [ ] **Step 1: Write the failing test**

Add to `TestResolveJobEndpoint` in `internal/handler/resolve_job_integration_test.go`:

```go
	t.Run("a vacancy we already carry answers found, and still records the board", func(t *testing.T) {
		// A storefront over an ATS we do NOT recognise, fronting a company whose posting the
		// catalogue already holds. The vacancy is not new, so the answer is found — but the
		// board behind the storefront may still be worth onboarding, so the contribution row
		// must be written before that answer is given.
		if _, err := pool.Exec(ctx, `
			INSERT INTO jobs (source, external_id, url, title, public_slug, company, company_slug, role_fingerprint)
			VALUES ('greenhouse', 'nimbus:41', 'https://boards.greenhouse.io/nimbus/jobs/41',
			        'Principal Java Architect', 'principal-java-nimbus', 'Nimbus', 'nimbus', $1)`,
			fingerprintOf(t, "Principal Java Architect", "nimbus")); err != nil {
			t.Fatalf("seed the crawled posting: %v", err)
		}
		const page = "https://careers.nimbus.test/jobs/principal-java-architect"
		resp, out := resolve(t, page, true)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200 — the catalogue already carries this vacancy", resp.StatusCode)
		}
		if out.Data == nil || out.Data.Status != "found" {
			t.Fatalf("body = %+v, want status found", out.Data)
		}
		if out.Data.PublicSlug == nil || *out.Data.PublicSlug != "principal-java-nimbus" {
			t.Errorf("slug = %v, want the canonical principal-java-nimbus", out.Data.PublicSlug)
		}
		var recorded int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM link_contributions WHERE url = $1`, page).Scan(&recorded); err != nil {
			t.Fatalf("read the contribution queue: %v", err)
		}
		if recorded != 1 {
			t.Errorf("contribution rows = %d, want 1 — a found vacancy does not excuse losing the board", recorded)
		}
	})
```

The page must parse AND must declare the company the canon is seeded under, or the import lands
in a different role cluster and the test fails for the wrong reason. Add a dedicated fixture
beside `secondMinderaPage` at the top of the file:

```go
// nimbusPage is a storefront vacancy for a company whose posting the catalogue already carries
// under a crawled source — the collapse case.
const nimbusPage = `<html><head><script type="application/ld+json">
{"@context":"https://schema.org","@type":"JobPosting",
 "title":"Principal Java Architect","description":"Own the platform.",
 "datePosted":"2026-07-21","jobLocationType":"TELECOMMUTE",
 "hiringOrganization":{"@type":"Organization","name":"Nimbus"}}
</script></head><body>Apply now</body></html>`
```

and route it in the `pages` map built inside `TestResolveJobEndpoint`:

```go
		"/jobs/principal-java-architect-nimbus": nimbusPage,
```

with the subtest's `page` const updated to match that path:
`https://careers.nimbus.test/jobs/principal-java-architect-nimbus`.

The fingerprint helper is package-local to `linkimport`, so `handler` needs its own. Add beside
the fixtures:

```go
// fingerprintOf builds the role fingerprint the import derives, so a seeded canon lands in the
// same role cluster as the imported page.
func fingerprintOf(t *testing.T, title, companySlug string) string {
	t.Helper()
	return jobhash.RoleFingerprint(db.UpsertJobParams{Title: title, CompanySlug: companySlug})
}
```

Import to add: `"github.com/strelov1/freehire/internal/jobhash"`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test -tags=integration ./internal/handler/ -run TestResolveJobEndpoint`
Expected: FAIL — the status is `review`, not `found`.

- [ ] **Step 3: Implement**

In `internal/handler/intake.go`, inside `Resolve`, extend the outcome switch so a deduped import
answers `found`. Place the new case FIRST, before `!imported`:

```go
	out := intakeOutcome{Board: intake.Board, CompanySlug: companySlug, Rewarded: rewarded}
	switch {
	case imported && res.Deduped:
		// The catalogue already carried this vacancy under a crawled source. Answering found
		// here, rather than at the top of Resolve, is deliberate: the contribution is recorded
		// first, because the board behind an unrecognised storefront may still be new.
		out.Status, out.PublicSlug = outcomeFound, res.PublicSlug
	case !imported:
		out.Status = outcomeQueued
	case intake.Tracked:
		out.Status, out.PublicSlug = outcomeTracked, res.PublicSlug
	case intake.Recognized:
		out.Status, out.PublicSlug = outcomeImported, res.PublicSlug
	default:
		out.Status, out.PublicSlug = outcomeReview, res.PublicSlug
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test -tags=integration ./internal/handler/ -run TestResolveJobEndpoint`
Expected: PASS, with every pre-existing subtest still green.

- [ ] **Step 5: Update the docs that state the outcomes**

- `internal/contribution/AGENTS.md`: the outcome list — note that `found` is reachable twice,
  once before any fetch (catalogue hit by URL) and once after an import that collapsed onto an
  existing posting.
- `web/src/lib/docs/api-spec.ts`: the `/jobs/resolve` description — `found` also answers a link
  whose vacancy the catalogue already holds under another source.
- Regenerate: `cd web && pnpm gen:api-docs` (writes `docs/API.md`).

- [ ] **Step 6: Full verification**

```bash
go build ./... && go vet ./... && gofmt -l internal cmd
go test ./...
go test -tags=integration ./internal/handler/ ./internal/linkimport/ ./internal/db/ ./internal/contribution/
cd web && pnpm lint && pnpm build
```
Expected: build/vet clean, `gofmt -l` silent, all tests pass, web lint 0 errors.

- [ ] **Step 7: Commit and open the PR**

```bash
git add internal/handler/ internal/contribution/AGENTS.md web/src/lib/docs/api-spec.ts docs/API.md
git commit -m "feat(intake): a link to a vacancy we already carry answers found"
git push -u origin feat/intake-sync-dedup
gh pr create --title "feat(intake): collapse a storefront link onto the posting it duplicates" \
  --body "Closes the gap PR #1244 left: a storefront over an ATS with no id-lookup (teamtailor,
lever, workable — every ATS but greenhouse and ashby) still wrote a second row for a vacancy the
crawl already carried. Two of the three duplicate pairs in prod are of that kind.

Before writing a weblink row, the import asks the catalogue for the open canonical posting of
the same role cluster. On a hit the row is still written — it is what makes the storefront URL
resolvable, since FindOpenJobByURL answers with the posting a duplicate duplicates — but marked
duplicate_of the canon and left out of the enrichment queue. The intake answers found with the
canonical slug, after recording the contribution: the board behind an unrecognised storefront may
still be new.

Design: docs/superpowers/specs/2026-07-29-intake-synchronous-dedup-design.md

Out of scope: pairs whose role_fingerprint has diverged (the aggregator copies), and the two
batch dedup passes sharing one duplicate_of column."
```

---

## Notes for the implementer

- The seeded `role_fingerprint` in Tasks 2 and 3 must equal what `write` derives, or the test
  seeds a cluster the import never joins and the assertion fails for the wrong reason. Both
  tests build it through `jobhash.RoleFingerprint` for that reason. If a test fails with
  `Deduped = false`, print both fingerprints before assuming the implementation is wrong.
- Do not "fix" the two batch dedup passes while here. They write to the same `duplicate_of`
  column by different criteria and must run in the order `cmd/reindex` runs them (role, then
  aggregator). Running one alone unmarks the other's work.
