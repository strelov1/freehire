## 1. Resolver package (`internal/companyname/`)

- [x] 1.1 Define `Resolver interface { Name(ctx context.Context, board string) (string, error) }` and a `registry map[string]Resolver` keyed by source name.
- [x] 1.2 Implement the shared careers-`<title>` parser (URL template + title-shape per ATS) covering BambooHR, Lever, Ashby, Pinpoint; extract `{Name}` from `Jobs at {Name} | {Name} Careers` and fall back to the `... Careers` trailing form.
- [x] 1.3 Implement API-field resolvers where exposed: Greenhouse board `name`. iCIMS needs none (already uses `HiringOrganization.Name`). Workday deferred — its POST/tenant request shape is a separate seam; the registry adds it without touching callers.
- [x] 1.4 Implement the acceptance gate `Accept(slug, candidate) (string, bool)`: `html.UnescapeString` → trim → reject empty/still-slug-like/test/recruiter titles → confidence match (squished-candidate ⊇ slug, or slug ⊇ squished-candidate, or word-initial acronym len ≥ 2 matches slug).
- [x] 1.5 Add a `SlugLike(name string) bool` helper (single lowercase token, no space, no uppercase) shared by the selection filter and tests.

## 2. Data access

- [x] 2.1 Add sqlc query `ListSlugLikeCompaniesForBackfill` returning company slug + name + a representative open job's source and URL for slug-like companies with ≥1 open job; regenerated `internal/db`.
- [x] 2.2 Add `RenameSlugCompany` that rewrites `jobs.company` + re-keys `company_slug` for a board's postings, then reuse existing `SyncCompaniesFromJobs` + `DeleteOrphanCompanies` for reconciliation.

## 3. Worker (`cmd/backfill-company-names/main.go`)

- [x] 3.1 Wire config (`DATABASE_URL`), pgx pool, and the sources HTTP client via `worker.Bootstrap`.
- [x] 3.2 Load eligible companies, dispatch to the per-source resolver via a bounded errgroup (24 concurrent) over the shared HTTP client.
- [x] 3.3 Apply accepted names through `RenameSlugCompany`; run the catalogue reconciliation at the end so a standalone run is self-contained.
- [x] 3.4 Add a `--dry-run` flag that prints proposed `slug -> name` renames without writing.
- [x] 3.5 Print a summary: resolved / applied / skipped-no-source / rejected counts + companies orphaned.

## 4. Tests

- [x] 4.1 Unit-test `SlugLike` and the acceptance gate against the pinpoint corpus cases (accepts AFC Bournemouth, GS1 Canada; rejects `kempinski`→recruiter, `joe-testing`, `mountainwarehouse`, `lbresearch`→Centellic rebrand, still-slug candidates).
- [x] 4.2 Unit-test the careers-`<title>` parser and entity decoding (title-tag extraction + `ExtractTitleName` + `Accept` decode cases).
- [x] 4.3 Unit-test the Greenhouse resolver and the registry lookup against a fake JSON getter.

## 5. Docs & rollout

- [x] 5.1 Add `internal/companyname/AGENTS.md` and a `cmd/backfill-company-names` entry to the root `AGENTS.md` command list + layout.
- [x] 5.2 Document the run recipe (`go run ./cmd/backfill-company-names` [`--dry-run`], then `make reindex`) and note slug/URL churn on rename.
- [x] 5.3 `go build ./... && go vet ./... && go test -race ./...` green. NOTE (operational, pre-live): run `--dry-run` against prod data and eyeball the proposed renames before the first live run.
