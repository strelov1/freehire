## Context

`cmd/harvest-boards` is the existing harvest tool: it takes a seed list of candidate board slugs, probes each against a provider's official public API (greenhouse/lever/ashby/…), and appends only the ones that return jobs to `sources/<provider>.yml` — "a validated fact set, not a redistributed dataset." Its weakness is the seed: a slug guessed from a company name rarely matches the real board (a collection-name harvest hit ~0.1% on greenhouse).

The collection datasets (`internal/collections`) carry each company's **website**. A spike confirmed that fetching a company's careers page and detecting the ATS link in its static HTML resolves the real board ~19% of the time — a far stronger signal than a guessed slug. This change adds the *discovery* half (website → real board) and feeds it into the existing *validation* half (`harvest-boards`).

## Goals / Non-Goals

**Goals:**
- Turn a company website into a validated ATS board (greenhouse/lever/ashby) via its careers page.
- Reuse `harvest-boards` for API validation + `sources/*.yml` writing — zero changes to it.
- Keep the discovery core (`internal/atsdetect`) pure and unit-tested.
- Target the companies we're missing (unmatched against our catalogue), to focus the fetch load.

**Non-Goals:**
- Headless/JS rendering — static HTML only; JS-only careers pages are skipped (documented seam).
- ATS platforms beyond greenhouse/lever/ashby (workday/icims/custom are a later pattern).
- Auto-deploy — the run produces a `sources/*.yml` diff the operator reviews before shipping.
- A standing/cron worker — this is a manual, periodic discovery run like `harvest-boards`.

## Decisions

**1. Two-step pipeline, harvest-boards reused unchanged.** `harvest-ats` discovers candidate `(provider, slug)` pairs and writes per-provider **seed** files; the operator then runs the existing `harvest-boards <provider> <seed>` to validate against the API and write `sources/*.yml`. *Alternative:* integrate validation into `harvest-ats` by extracting the probers into an importable `internal/` package — rejected for v1 as scope creep (touches the working harvest-boards); the two-step is clean separation (discover ≠ validate) and the seeds are small, so the validation pass is fast.

**2. `internal/atsdetect` is the pure, testable core.** `Detect(html string) (provider, slug string, ok bool)` scans for the known ATS URL shapes — `boards.greenhouse.io/<slug>` and the `embed/job_board?for=<slug>` variant, `jobs.lever.co/<slug>`, `jobs.ashbyhq.com/<slug>` — returning the first match (provider precedence fixed). It never fetches; all I/O lives in the command. Slugs are validated to a sane shape (lowercase alphanumeric/hyphen) so a malformed capture is dropped.

**3. Career-page discovery is a small fixed set of fetches.** Per company: the homepage, `/careers`, `/jobs`, `/career`, plus one careers/jobs link discovered on the homepage (an `href` whose text/last path segment is careers/jobs). First page that yields a detected ATS wins; the rest are skipped. Fetches go through the `internal/sources` HTTP client (429 backoff, redirects, timeout).

**4. `extract` filters to unmatched and reuses dataset URLs.** It reads the dataset URLs from `collections.All` (single source of truth for where the data lives) but parses each dataset for `(name, website)` itself (the collections name-parsers don't carry websites; the website field differs per dataset — yc `website`, european `Website URL`, fortune500/techstars CSV columns). It drops any company whose `normalize.Slug(name)` is in a supplied prod company-slug set, emitting the rest as JSON.

## Risks / Trade-offs

- **Static-only misses JS-rendered careers pages** → Accepted (the spike's 19% is static-only and still ~190× the slug-probe); headless is a documented future seam behind the same `Detect` interface.
- **Heavy outbound fetch load** (one company = up to ~5 page fetches; thousands of companies) → bounded concurrency + per-request timeout + the shared client's 429 backoff; unmatched-only roughly halves the set; best-effort (per-company failures are logged and skipped, never abort the run).
- **Bot-blocking / Cloudflare on some sites** → those companies just resolve to nothing and are logged; no retry storm.
- **A careers page can link a stale/empty board** → the second step (`harvest-boards`) validates against the provider API and drops boards with no jobs, so a stale link never reaches `sources/*.yml`.

## Migration Plan

This is additive tooling; nothing to migrate. Operational run:
1. Export the prod company-slug set: `ssh … "psql -tAc 'SELECT slug FROM companies'" > companies.txt`.
2. `go run ./cmd/harvest-ats extract companies.txt > unmatched.json`.
3. `go run ./cmd/harvest-ats resolve unmatched.json` → `*.seed.json` per provider.
4. `go run ./cmd/harvest-boards <provider> <provider>.seed.json` for greenhouse/lever/ashby.
5. Review the `sources/*.yml` diff; deploy sources (`deploy.sh --no-build`) + ingest.
- **Rollback:** the run only writes local seed/source files reviewed before deploy; nothing reaches prod without the operator shipping the diff.

## Open Questions

- Final list of careers-page paths to try (start with the fixed set above; widen if the live run shows common misses).
- Whether to also parse the unicorn `Company Website` column (often empty in the snapshot) — include opportunistically, skip when blank.
