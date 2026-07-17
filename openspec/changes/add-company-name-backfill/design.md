## Context

ATS adapters set `Job.Company = e.Company` from the board file (`internal/sources/*.go`; iCIMS is the sole exception, preferring `HiringOrganization.Name`). A large fraction of `sources/*.yml` entries carry a squished slug in `company:`, so the slug becomes the canonical company name. The `companies` table is derived from `jobs` (`SyncCompaniesFromJobs`: `slug = company_slug`, `name = company`; `DeleteOrphanCompanies` sweeps unreferenced rows). The SPA resolves logos via logo.dev's *name* endpoint (`web/src/lib/logo.ts`), which 404s on squished single-token names and degrades to a monogram.

Pinpoint was fixed manually (PR #825): 105 names harvested from careers-page titles (`Jobs at {Name} | {Name} Careers`), decoded, and gated by a confidence check. This change generalizes that ad-hoc process into a reusable worker rather than repeating hand-edits across ~20k slug entries (bamboohr ~7223, workday ~5029, ashby ~2712, lever ~1624, …).

## Goals / Non-Goals

**Goals:**
- Replace slug-like company names with real display names sourced deterministically from each ATS's native title/API.
- Scale across ATSes via a small per-ATS resolver interface; make adding a resolver cheap.
- Be safe: touch only slug-like names on boards with live jobs; never invent a name; require a confidence match; be idempotent.
- Be observable: report resolved / skipped / rejected counts.

**Non-Goals:**
- No change to display, `companies` table shape, or logo-resolution logic — only the stored name value improves.
- No prettify-the-slug fallback (`afcb` → "Afcb"): out of scope, violates dict-only spirit.
- No adapter refactor to read API names at ingest time — that is a separate, complementary change; this worker is the backfill for boards where the API gives nothing (Lever/BambooHR) and a catch-up for the rest.
- No new schema/migration.

## Decisions

- **Run-once worker + resolver package.** `cmd/backfill-company-names/main.go` orchestrates; `internal/companyname/` holds the strategies and the acceptance gate. Mirrors existing run-once workers (`cmd/import-yc`, `cmd/backfill-derive`) and keeps HTTP/parse logic unit-testable without a DB.
- **Resolver interface keyed by source.** `Resolver interface { Name(ctx, board string) (string, error) }` with a registry `map[source]Resolver`. Title-based resolvers (BambooHR, Lever, Ashby, Pinpoint) share one careers-`<title>` parser differing only by URL template and title shape; API resolvers (Greenhouse board `name`, Workday) parse a field. Sources without a resolver are skipped, not guessed.
- **Selection query.** Add a read query for companies whose `name` is slug-like (`name !~ '[[:space:][:upper:]]'` and single token) AND `EXISTS` an open job on the slug. Prefer doing the slug predicate in SQL to keep the worker's working set small; final slug classification still validated in Go to stay authoritative.
- **Acceptance gate (shared with the manual pinpoint pass).** `html.UnescapeString` → trim → reject empty / test / recruiter titles → confidence: squished-candidate contains the slug, or slug contains squished-candidate, or the candidate's word-initial acronym (len ≥ 2) matches the slug. Reject otherwise. This is the exact logic validated on the 241 pinpoint boards (104 accepted, 10 correctly rejected).
- **Application path.** Update `jobs.company` for the board's postings (so the derived catalogue re-keys), then rely on the existing `SyncCompaniesFromJobs` + `DeleteOrphanCompanies` reconciliation the ingest already runs; the worker may invoke the sync queries at the end so a standalone run is self-contained. Renaming can change `company_slug`, which is expected (the public URL follows the real name) and identical to how PR #825 behaves on next ingest.
- **Concurrency + politeness.** Bounded worker pool over the sources HTTP client (proxy/User-Agent aware, like the manual pass at ~24 concurrent), so large ATSes complete in reasonable wall-clock without hammering a single host.
- **Prioritization by live jobs.** The `EXISTS open job` filter means the tens-of-thousands slug count collapses to the boards that actually surface in the UI; dead boards cost nothing.

## Risks / Trade-offs

- **Subdomain reuse / rebrands** can yield a wrong-but-plausible title (e.g. `mountainwarehouse` → "Mountain Group", `doradosoftwaregroup` → "Optitex"). The confidence gate catches unrelated names, but same-prefix mismatches can slip through. Mitigation: keep the gate conservative, log low-confidence/near-miss decisions, and treat the worker's output as reviewable (dry-run mode that prints proposed renames without writing).
- **Title-format drift.** Careers-page title shapes vary and change over time; a resolver that stops matching simply yields no name (safe no-op), surfaced in the summary rather than corrupting data.
- **Slug churn on rename.** Changing a name re-keys `company_slug`, orphaning the old `/companies/<old-slug>` URL. Accepted: it matches the rebrand reality and the existing PR #825 behavior; sitemaps/redirects are out of scope here.
- **Cost of native fetches.** One HTTP round-trip per eligible board; bounded by the live-jobs filter and concurrency. Acceptable for a run-once maintenance job.
- **Overlap with a future ingest-time adapter fix.** If adapters later read API names at ingest, this worker's role shrinks to title-only ATSes; the resolver package is shared, so no wasted work.
