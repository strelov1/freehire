## Context

The full reconnaissance record lives in
`docs/superpowers/specs/2026-07-28-opencats-adapter-harvest-design.md`; this document carries
the decisions that shape the implementation.

Every provider crawled today is multi-tenant SaaS: one adapter plus a slug list yields
thousands of boards. OpenCATS inverts that — a self-hosted PHP ATS, one install per company,
on the company's own domain. `cmd/harvest-boards` resolves candidates either from a seed slug
list or from a provider API (`resolveCandidates`, `main.go`), and neither exists here.

Measured on 2026-07-28: 78 candidate hosts from urlscan.io yielded **9 live portals with 260
open jobs**, all reachable over HTTPS. Common Crawl was tried and rejected — its CDX index is
sorted by SURT (domain first), so a global search by URL path returns nothing.

## Goals / Non-Goals

**Goals:**
- Crawl self-hosted OpenCATS portals as an ordinary provider, inheriting `board_health`,
  cooldown, the unseen-job sweep, and incremental search indexing without special-casing.
- Make discovery repeatable, so installs that appear later are found by re-running the tool
  rather than by remembering to look.
- Survive template drift: an install that rewrites its portal must degrade to zero jobs, not
  to garbage jobs.

**Non-Goals:**
- Application submission — the adapter is read-only, as all adapters are.
- FreeATS — zero public installs found; the seam is recorded, no code is written.
- Self-closing support — withdrawn postings close via the 48-hour sweep.
- Exhaustive discovery — urlscan knows only hosts someone has scanned.

## Decisions

**Board is host plus optional path prefix, not a new board-file field.**
Installs mount the portal differently: `atscareers.g4s.com` at the web root,
`careers.boomit.pt/careers` nested. Encoding both in the existing `board` string keeps the
board-file format untouched and reuses the shape `sources/catsone.yml` already proves in
production, where `board` is a careers host and is sometimes a customer's own domain
(`jobs.evoplay.com.ua`). *Alternative rejected:* a separate `path` field in the board file —
changes a format shared by ~150 providers to serve one.

**Parse routing invariants, never markup.**
G4S rewrote the template (HTML5, custom assets); boomit and crewlogix run stock XHTML 1.0;
indovision added "education" and "experience" columns. CSS classes, column order, and column
count agree nowhere. Two things hold everywhere: postings link as
`index.php?m=careers&p=showJob&ID=<n>`, and the anchor's text is the job title. Location and
description therefore come from the labelled detail page, never from a positional listing
column. *Alternative rejected:* per-install parsing profiles — unbounded maintenance for a
260-job provider.

**Let the pipeline namespace the id.**
An OpenCATS `ID` is unique only within one install — `ID=24` exists on both boomit and
crewlogix — so under `jobs UNIQUE (source, external_id)` with a shared `source` these would
overwrite each other. No adapter-side handling is needed: `pipeline.jobIdentity`
(`internal/pipeline/pipeline.go:539`) already routes every id through
`sources.NamespaceExternalID(e.Board, j.ExternalID)`. The adapter emits the raw id.
*Alternative rejected:* building a composite id inside the adapter — would double-namespace
and diverge from the link-source path that dedups against the same key.

**Discover from urlscan.io, in the harvest tool only.**
The stock `<title>` is the richest signature (66 hosts) because administrators change the
portal path far more often than the page title; URL-routing signatures add the rest. The
dependency is confined to `cmd/harvest-boards`, a run-once host tool — no production worker
reaches urlscan, so an outage there cannot affect ingest. *Alternatives rejected:* Common
Crawl (cannot search by path), Censys/Shodan (API key, tight free tier), search-engine dorks
(no bulk API, ends in captchas).

**Exclude `*.catsone.com` at discovery.**
Commercial CATS shares the URL scheme and is already crawled under its own provider. A shared
posting admitted under both providers is a cross-provider duplicate, which the
`(source, external_id)` key cannot detect — dedup is scoped within a source.

## Risks / Trade-offs

- **Small absolute volume (260 jobs).** → Justified by marginal cost, not by count: one
  adapter plus one prober, every pipeline facility reused. If the harvest had found fewer
  than 5 boards the change would have been dropped; it found 9.
- **Template drift breaks an install's parse.** → Routing-only parsing; a broken install
  yields zero jobs, and `board_health` cools it off after 3 consecutive failures rather than
  emitting malformed postings.
- **urlscan changes or rate-limits its API.** → Host-tool only; a failed discovery run
  reports and exits, ingest is untouched.
- **Discovery ceiling mistaken for exhaustion.** → Recorded in the spec: an empty future
  harvest means "no newly *scanned* installs", not "no new installs".
- **Industry mix is broader than tech** (e.g. `opencats.gorgany.com` is an outdoor retailer).
  → No special handling; the existing `is_tech` enrichment gate already governs this.
- **Portal pages are un-escaped by upstream's own admission** (`modules/careers/CareersUI.php`
  carries an escaping TODO). → We read and sanitise; description HTML goes through the shared
  sanitiser, as every HTML adapter already does.

## Migration Plan

No schema change, no migration, no backfill. Deployment is the ordinary path: merge, then add
a cron schedule for `sources/opencats.yml` alongside the other provider schedules. Rollback is
removing the schedule; jobs already ingested close through the normal sweep.

## Open Questions

None. Scope, threshold, and discovery channel were settled before implementation.
