## Context

See proposal.md for motivation (freehire#1636). This section records what live inspection of
joppy.me (2026-09-15) actually found, since the issue's own notes leave several shapes open.

- `robots.txt` is `User-agent: * / Allow: /`; the declared `Sitemap:` is `sitemap.xml`, a
  sitemap **index** listing three children: `sitemap.static.xml`, `sitemap.directory.xml`
  (the one that matters here) and `sitemap.newsletters.xml`.
- `sitemap.directory.xml` lists both `/companies/<slug>` (one per company, 194 live) and
  `/companies/<slug>/<uuid>` (one per open posting, currently ~59 across ~25 companies with
  any open posting) — confirmed live: every company checked has exactly as many
  `/companies/<slug>/<uuid>` sitemap entries as it has entries in its own page's jobs array
  (9 for `seidor`, 8 for `indramind-cybersecurity`).
- `GET /companies/<slug>` is a Next.js **Pages Router** app (not the App Router RSC-flight
  shape the issue's "hh/bayt" comparison might suggest): it embeds a `<script
  id="__NEXT_DATA__">` JSON blob whose `props.pageProps.company.jobs` array holds every one of
  that company's currently open postings **in full** — title, complete HTML description,
  structured skills (each flagged mandatory or not), a `place` object (remote/hybrid/office
  booleans, a free-text `located` string, and a `cities` list), required languages with a 1-5
  numeric level, `isSalaryPublic` plus `salaryMin`/`salaryMax` (present internally even when
  not public — must be gated on the flag), `sponsorVisa`, `relocationPack`, and
  `onlyEuCandidates`. No separate per-posting request is needed or possible for anything the
  page doesn't already carry.
- No currency field anywhere: the platform is Spain-only, and every salary figure observed
  reads as annual EUR (bare integers in the 25k-60k range, consistent with Spanish gross
  annual pay, never a monthly or hourly figure).
- `internal/ingest/sources/talenthr.go` (and `alignerr.go`) already extract a page's
  `__NEXT_DATA__` blob via the shared `bracketSlice(body, "__NEXT_DATA__", '{', '}')` helper —
  Joppy reuses that helper rather than introducing a second way to find the same script tag.
- `internal/ingest/sources/getmanfred.go` is the closest existing shape for a **boardless**
  Spanish marketplace adapter (`boardless()` + `aggregator()` markers, company taken from each
  posting) — Joppy follows the same two markers, but not its list-then-per-offer-detail
  fan-out, since Joppy's "list" (one company page) already IS the detail for every posting on
  it.

## Goals / Non-Goals

**Goals:**
- Crawl every company in the directory each run and return every open posting found, cheaply
  (well under 300 requests for a board this size).
- Map every field the platform states in a structured form onto the matching `Job` field, and
  never drop a stated fact just because `Job` has no dedicated column for it.
- Never publish a salary the employer did not opt to make public, even though the platform's
  own data holds it internally either way.

**Non-Goals:**
- No CEFR-precise `EnglishLevel` mapping from Joppy's own 1-5 language scale (see Decisions —
  rejected for lack of an authoritative equivalence).
- No `HydratingSource`/seen-based incremental fetch: nothing is expensive enough here to bound
  by novelty, unlike justjoin/getro. Every run is a full re-read of the whole directory.
- No `fullBoardListing` marker in this change — that classification needs its own audit
  (`internal/ingest/sources/AGENTS.md`'s bar: verified proof of reaching the source's natural
  end, not just "looks complete today"), and 25-40 active companies is too small a sample to
  earn it confidently in the same change that first onboards the source.

## Decisions

**Crawl ALL 194 company slugs from the sitemap, not just the ~25-37 with a sitemap job
sub-entry.** The job sub-entries look like a free prefilter (fetch only companies known to
have openings), but trusting them risks the same class of bug this codebase has hit before
(Workstream's sitemap carrying no `/j/` URLs at all, Profession.hu's truncated
`sitemap-listings-itdev-hu.xml`): if a job sub-entry lags its company page by even one crawl
cycle, a real opening is silently skipped. 194 lightweight page fetches per run is cheap for a
board this size (comparable to Profession.hu's 82-page IT-slice walk), so there is no cost
reason to take the risk.

**Fan out per-company page fetches with a small bounded worker pool, not the shared
`fetchDetails` helper.** `fetchDetails[P](postings []P, workers int, fetch func(P) (Job,
bool))` is 1:1 — one list item yields at most one `Job`. Joppy's shape is 1:N — one company
page yields zero or more `Job`s. Reusing `fetchDetails` would mean fetching a company page once
per posting on it (up to 9x duplicate work for `seidor`) just to fit the existing helper's
contract, which is worse than writing the ~15-line 1:N variant this adapter actually needs.

**Work mode is derived by priority: `isHybrid` wins whenever set, else `isRemote` XOR
`isOffice`, else unset.** Live samples show the three flags are not mutually exclusive
(`(remote=true, hybrid=true, office=true)` was observed). `isHybrid` is the platform's own
explicit "hybrid" statement and is trusted outright. When it is false, exactly one of
`isRemote`/`isOffice` being true is unambiguous; both true (with hybrid false) has no clean
single-word reading and is left `""` so the pipeline's location-text heuristic decides, rather
than guessing between "remote" and "onsite" for what the platform itself did not disambiguate.

**`EnglishLevel` is left unset; the required language(s) and stated level are folded into the
description text instead.** Joppy exposes only four UI labels ("Basic"/"Intermediate"/
"Fluent"/"Native") over what is observably a five-point numeric scale (1-5 seen for Spanish).
`internal/ingest/sources/profession.go`'s own English-level mapping is trusted only because
Hungary's alapfok/középfok/felsőfok levels have a government-decreed CEFR equivalence
(137/2008) — no such authoritative equivalence exists for Joppy's own scale, and inventing one
(is a "3" a B1 or a B2?) is exactly the "reading them by their everyday sense puts each posting
a level off" trap that decision's own comment warns against. The requirement is real and
non-fabricated (folded into the description), just not forced into a vocabulary the platform
never actually committed to.

**Salary currency is hardcoded `"EUR"`, period `"year"`.** The platform publishes no currency
field at all, but the board is Spain-only (confirmed: every `located`/`cities` value across the
sample is Spanish) and every salary figure reads as an annual gross range in the shape Spanish
job postings use — unlike Workstream (US+Canada, ambiguous `$`) this board has no cross-country
ambiguity to hedge against.

**Visa sponsorship, relocation package, and EU-candidates-only are appended to the description
as plain sentences, not dropped.** `Job` has no dedicated field for any of the three. This
mirrors Workstream's pay-line and remote.com's timezone-suffix precedent: a real, structured
platform signal with nowhere to go structurally still reaches the stored posting as text rather
than being thrown away.

## Risks / Trade-offs

- [194 page fetches/run for ~25-40 companies with real openings] → acceptable at this board's
  scale (~80 postings); revisit only if Joppy's directory grows by an order of magnitude.
- [No authoritative English-level mapping] → the requirement still reaches search via full-text
  description matching (`internal/job/jobfacts`'s English-prose fallback), just not via the
  structured `EnglishLevel` facet — a real loss on a non-English-titled board, accepted
  deliberately over publishing a guessed CEFR level.
- [No `fullBoardListing` marker yet] → the board is swept by the ordinary 48h unseen-job rule
  rather than closed on proof of a complete walk; acceptable for a first onboarding, revisit
  once the crawl has run in prod long enough to audit against the AGENTS.md bar.

## Migration Plan

No migration. After merge: register `joppy` in `sources.All`, then (operationally, not part of
this PR) `cmd/add-board --provider=joppy --apply` once to seed the single boardless catalog row
so `cmd/ingest joppy` has something to crawl and `deploy/bin/gen-ingest-timers.sh` schedules it.
Rollback is deleting that one catalog row plus reverting the registry line — no stored data
depends on the adapter existing.
