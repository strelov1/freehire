## 1. Schema

- [x] 1.1 Add migration: `companies.company_info_wikipedia_checked_at timestamptz NULL`.
- [x] 1.2 Run `make sqlc` and confirm the generated Go picks up the new column with no other diff.

## 2. Wikidata client

- [x] 2.1 Add a small Wikidata HTTP client package (under the `job` block, alongside `internal/job/ycdir`) wrapping `wbsearchentities` (candidate search by name) and the `query.wikidata.org` SPARQL endpoint (type-confidence `ASK` query), with a fixed request rate and backoff-on-429/5xx.
- [x] 2.2 Curate the initial business/organization QID anchor set (`Q4830453` business, `Q43229` organization, `Q6881511` enterprise, `Q783794` company, `Q891723` public company, `Q328664` corporation, plus common subtypes worth anchoring directly) as a package-level constant.
- [x] 2.3 Implement the type-confidence check: given a candidate QID, `ASK` whether `wdt:P31/wdt:P279*` reaches any anchor QID.
- [x] 2.4 Implement fetching the accepted candidate's Wikidata `description` (→ `tagline`) and, via its `enwiki` sitelink, the Wikipedia summary `extract` (→ `company_info.summary`).
- [x] 2.5 Unit-test the client against recorded fixtures for: a clean company match, a same-named person/place/concept rejection (using the spike's own false-positive cases as fixtures), and a subtype match that a flat `P31` check would miss (e.g. "defense contractor").

## 3. Backfill worker

- [x] 3.1 Add the `companies` query: select candidates where `tagline IS NULL AND company_info_wikipedia_checked_at IS NULL`, keyset-paginated, bounded by a per-run max.
- [x] 3.2 Add the `companies` write: on a confirmed match, set `tagline`, merge `company_info.summary` (gap-fill semantics — never overwrite an existing key), set `company_info_at` and `company_info_wikipedia_checked_at`; on no match or a rejected match, set only `company_info_wikipedia_checked_at`.
- [ ] 3.3 Implement `cmd/backfill-company-info-wikipedia`: dry-run (report proposed writes) by default, `--apply` to write; `WIKIPEDIA_BACKFILL_MAX_PER_RUN` env cap via `worker.EnvInt64`.
- [ ] 3.4 Wire the worker through `internal/platform/worker` bootstrap (`DATABASE_URL` only, no other required env).

## 4. Verification

- [ ] 4.1 Integration test (`-tags=integration`) against a seeded `companies` table: a company with an existing tagline is untouched; a company with no tagline and a fixture-backed confident match gets filled; a company with no tagline and a fixture-backed rejected match gets only its checkpoint column set; a re-run performs no further writes or lookups for already-checked rows.
- [ ] 4.2 Dry-run the worker against production with a small `WIKIPEDIA_BACKFILL_MAX_PER_RUN`; manually review the proposed matches against the spike's known-good sample (Hitachi Energy, Sberbank, Nissan, Masco, Paladin Energy, etc.) and known-bad sample (`Boardroom Appointments`, `CWAN`, `Evolution`, `takeaway`) before enabling `--apply` at scale.
- [ ] 4.3 `gofmt -l .`, `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...` all clean.

## 5. Rollout

- [ ] 5.1 Run `--apply` against production at increasing scale, staged like `merge-companies` (small bound first, widen once reviewed).
- [ ] 5.2 Add the worker to the deploy host's cron schedule (periodic, e.g. monthly) per `deploy/AGENTS.md`, so newly-crawled companies keep getting checked.
- [ ] 5.3 Document the new worker in the root `AGENTS.md` worker list (env vars, schedule, idempotency notes) alongside the other one-off/periodic backfills.
