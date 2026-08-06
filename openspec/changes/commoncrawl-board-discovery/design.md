## Context

`cmd/harvest-boards` already has an opt-in `discoverer` capability (`prober.go`): a provider's
prober implements `discover(ctx, c) ([]string, error)` and, when the tool runs with no seed
file, those candidates take the place of a seed list. `gupyProber` discovers by paging Gupy's
own global jobs feed; `opencatsProber` discovers by querying a public URL-scan index
(`urlscan.io`) for OpenCATS's page-title and routing signatures, because OpenCATS is
self-hosted across arbitrary domains and has no tenant catalogue of its own to page.

Common Crawl was evaluated once before, for that same OpenCATS design
(`docs/superpowers/specs/2026-07-28-opencats-adapter-harvest-design.md`), and rejected: its CDX
index is SURT-sorted (domain first), so a query can't search by page signature across arbitrary
domains. Greenhouse and Ashby are the opposite shape — every tenant board lives under one known
host (`boards.greenhouse.io`, `jobs.ashbyhq.com`), which is exactly the query CDX is built for
("every URL Common Crawl has seen under this host"). The prior rejection doesn't apply here.

A manual spike (curl against `index.commoncrawl.org`, 2026-08-06) confirmed this: a single
recent snapshot returned 1789 unique Greenhouse company slugs (412 not already in
`sources/greenhouse.yml`) and 407 unique Ashby slugs, both cleanly extractable as the first path
segment. The same spike found `jobs.lever.co/robots.txt` disallows `CCBot` entirely, so no
Lever board ever reaches the CDX index — Lever is out of scope for this change.

## Goals / Non-Goals

**Goals:**
- Add Common Crawl CDX discovery for Greenhouse and Ashby, wired through the existing
  `discoverer` interface so the rest of the pipeline (live-validate, dedup, append) needs no
  changes.
- Keep the snapshot list current automatically (no hardcoded snapshot ids to rot).
- Degrade gracefully: one bad snapshot fetch shouldn't sink the whole discovery run.

**Non-Goals:**
- Lever discovery (ruled out by the spike — CCBot is blocked).
- Any change to how a *seeded* run behaves — supplying a seed file still bypasses discovery
  exactly as it does today.
- Scheduling this as a cron job. `harvest-boards` is a manually-run host tool; this stays that
  way.

## Decisions

**One shared CDX helper, not per-provider duplication.** Greenhouse and Ashby differ only in
which host they query and how a raw slug maps to a board id — both use the URL's first path
segment as-is (neither provider is a `seedMapper`, per the existing prober.go convention). A
single `commonCrawlCandidates(ctx, c, hostPrefix string) ([]string, error)` in a new
`cmd/harvest-boards/commoncrawl.go` does the snapshot lookup, paging, and slug extraction; each
prober's `discover` method is a one-line call into it. Considered: writing `discover` separately
per provider — rejected, it would duplicate the snapshot-fetch and pagination logic for zero
behavioral gain.

**Snapshot list from `collinfo.json`, latest 3, not hardcoded.** `index.commoncrawl.org/collinfo.json`
lists every available snapshot with its own CDX API root; the helper takes the first 3 entries
(the list is newest-first). Considered: hardcoding a snapshot id — rejected, Common Crawl cuts a
new snapshot roughly monthly, so a hardcoded id goes stale within weeks and the tool would
silently query a dead/archived index. Considered: querying *all* available snapshots — rejected,
snapshots a few months apart overlap heavily in which companies they've seen, so the marginal
new-candidate yield per additional snapshot drops fast while cost (CDX requests, page count)
keeps climbing; 3 was the number the reference implementation (job-hunter) validated in
production use, and the spike didn't have time budget to establish a better number empirically.

**Slug extraction: first non-empty path segment, lowercased.** Matches how `resolve()` and the
existing probers already treat a board id for these two providers (case folding only applies to
Workday via `dedupKeyer`; Greenhouse/Ashby board ids are the literal path segment). No special
handling for query strings (they're not part of the URL path) or for known non-board paths like
Ashby's `/meeting/` or `/b/` — those are simply slugs that will fail live-validation like any
other invalid candidate, since the probe pipeline already treats "0 jobs / unreachable" as
"skip, don't append." No extra filtering logic pays for itself here.

**Per-snapshot page cap, not a hard total cap.** Mirrors `gupyMaxOffset` — bound the number of
CDX pages read per snapshot so a snapshot whose index never terminates (or a paging bug) can't
loop forever. Unlike Gupy, the *number* of pages CDX will return for one host is small and
knowable ahead of time (`showNumPages=true` returned `pages: 1` for both Greenhouse and Ashby in
the spike), so the cap is a safety backstop, not an expected limit in normal operation.

**Errors: one snapshot failing is not fatal, all snapshots failing is.** Mirrors
`opencatsProber.discover`'s existing rule ("all N signature queries failed" is the only hard
error). A single 504 from `index.commoncrawl.org` (observed during the spike — the endpoint is
occasionally flaky under an unbounded query) logs and moves to the next snapshot; if every
snapshot fails, `discover` returns an error so the run reports it plainly instead of silently
producing zero candidates.

## Risks / Trade-offs

- **[Risk] `index.commoncrawl.org` is a free public service with no SLA and was observed to
  504 under load during the spike.** → Mitigation: per-snapshot error tolerance (above), plus
  the tool is run manually/on-demand, so a bad run costs nothing but a retry.
- **[Risk] Low live-hit rate (~4% of new candidates in the spike sample had open jobs right
  now).** → Not a defect to fix — it's inherent to discovering from a month-old crawl snapshot,
  and the existing live-validation step already exists precisely to filter it. Still net
  positive: ~412 new Greenhouse candidates from one snapshot yielded roughly a dozen-plus live
  boards for the cost of API calls the pipeline already budgets for seeded runs.
- **[Risk] Greenhouse has visibly migrated its web frontend from `boards.greenhouse.io` to
  `job-boards.greenhouse.io` (301 redirects seen throughout the spike sample).** → No mitigation
  needed: the harvest validates against `boards-api.greenhouse.io` (a separate, stable API host,
  already what `greenhouseProber.probe` uses today), not the web frontend, so the redirect is
  irrelevant to slug validity. Worth a one-line comment in the code so a future reader isn't
  alarmed by the redirect status seen in raw CDX records.

## Migration Plan

No migration — this adds a capability to a manually-run host tool with no persisted state of
its own. First real run is invoked by hand after merge; nothing changes automatically.

## Open Questions

None — the spike resolved the two open questions from the design brainstorm (does Common Crawl
work for these domains at all, and does Lever work). Snapshot count (3) is a judgment call
carried over from the reference implementation rather than independently re-derived; if a real
run's yield looks thin, revisit that number rather than the overall approach.
