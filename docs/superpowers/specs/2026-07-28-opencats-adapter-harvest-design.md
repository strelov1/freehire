# OpenCATS source adapter and portal harvest

Date: 2026-07-28
Status: approved design, not yet implemented

## Problem

`freehire` crawls ~150 ATS providers, all of them multi-tenant SaaS: one adapter plus a list
of slugs yields thousands of boards. OpenCATS is the opposite shape — a self-hosted PHP ATS
that each company installs on its own domain. There is no tenant catalogue to enumerate, so
the platform is invisible to the existing harvest, which resolves candidates either from a
seed slug list or from a provider API (`resolveCandidates`, `cmd/harvest-boards/main.go`).

The question this design answers is whether self-hosted OpenCATS installs carry enough live
postings to be worth a provider at all, and if so, how to find them repeatably.

## Reconnaissance (completed 2026-07-28)

Candidate discovery through the urlscan.io search API, by signature:

| Query | Results | Unique hosts |
|---|---|---|
| `page.title:"OpenCATS"` | 144 | 66 |
| `page.url:"careers/index.php"` | 32 | 19 |
| `page.url:"m=careers"` | 17 | 13 |

The default `<title>` is by far the richest signal: most administrators change the portal
path or put it behind a rewrite, but leave the page title alone.

Probing the 78 deduplicated candidate hosts found **9 live portals carrying 260 open jobs**:

| Host | Jobs |
|---|---|
| `atscareers.g4s.com` | 108 |
| `itsource.indovisionglobal.com` | 95 |
| `opencats.gorgany.com` | 33 |
| `careers.crewlogix.com` | 8 |
| `careers.boomit.pt` | 6 |
| `rms.adgonline.ca` | 5 |
| `dwrs.mggroup.lk` | 2 |
| `ats.thomasstewartbuilders.com` | 2 |
| `opencats.vikisoft.com.ua` | 1 |

All nine answer over HTTPS, so the adapter is https-only — no cleartext fallback.

Common Crawl was evaluated and rejected as a discovery channel: its CDX index is sorted by
SURT (domain first), so a global search by URL path returns nothing.

FreeATS is **out of scope**. Its README documents only self-hosted Docker deployment, with
no public career-page pattern or API, and a urlscan sweep finds zero public installs
(`free-ats.ru` and `freeats.ca` are unrelated companies of a similar name). The seam is
noted here; no code is written for it.

## Design

### Board identity

`board` is the portal root: host plus an optional path prefix, because installs differ in
where the portal sits — `atscareers.g4s.com` serves it at the web root while
`careers.boomit.pt/careers` nests it. The listing URL is
`https://<board>/index.php?m=careers&p=showAll`.

This reuses the shape `sources/catsone.yml` already proves in production, where `board` is a
careers host and is sometimes a customer's own domain (`jobs.evoplay.com.ua`). No change to
the board-file format.

### Dedup key

An OpenCATS job `ID` is unique only within one install: `ID=24` exists on both
`careers.boomit.pt` and `careers.crewlogix.com`. Under `jobs UNIQUE (source, external_id)`
with a shared `source = "opencats"` these would collide and overwrite each other.

No special handling is required in the adapter. `pipeline.jobIdentity`
(`internal/pipeline/pipeline.go:539`) routes every id through
`sources.NamespaceExternalID(e.Board, j.ExternalID)`, producing `<board>:<id>`. The adapter
emits the raw numeric id and the pipeline namespaces it — the same single door used by
link-source resolution, so a job reached by either path dedups against the other.

### Parsing strategy: routing, not markup

Installs customise the template freely. G4S has rewritten it (HTML5, custom assets),
boomit and crewlogix run the stock XHTML 1.0 template, and indovision has added "education"
and "experience" columns. CSS classes, column order, and column count agree nowhere.

Two invariants hold across every install inspected, and they are sufficient:

1. A job links as `index.php?m=careers&p=showJob&ID=<n>` — this yields `ExternalID`.
2. **The anchor's text is the job title** — verified on all four installs inspected.

Location and description therefore come from the detail page, never from the listing row:
row columns are positional and differ per install, while detail-page fields are labelled.

### Components

**`internal/sources/opencats.go`** — the adapter, modelled on `catsone.go`. `Fetch` reads the
listing, collects `(id, title, detail URL)` per anchor, then fans out over detail pages via
the existing `fetchDetails` helper at `defaultDetailWorkers`. Registered as one line in
`sources.All`. Description passes through `sanitizeHTML`.

**`cmd/harvest-boards/opencats_prober.go`** — a `prober` that also implements the existing
`discoverer` interface, so `go run ./cmd/harvest-boards opencats` runs with no seed file.
`discover` queries urlscan for the signatures above, unions and deduplicates hosts, and
drops noise. `probe` tries the known portal paths, counts `showJob` anchors, and returns
`("", 0, nil)` for anything absent or empty — a silent skip, never fatal.

The prober MUST exclude two classes, both observed in the real data:

- **`*.catsone.com` hosts.** Commercial CATS shares the URL scheme with its open-source
  descendant and is already covered by `sources/catsone.yml`. Admitting them creates a
  cross-provider duplicate, which the `(source, external_id)` key cannot catch.
- **Non-job anchors**, e.g. boomit `ID=24` "Can't find what you're looking for? Apply here",
  a general-application form rather than a position.

**`sources/opencats.yml`** — a new provider file, one entry per validated board, following
the one-file-per-provider convention. The company name is proposed by the prober from
`<title>` with a domain fallback, then corrected by a human reviewing the harvest diff.

### Failure handling

A listing that will not load is a board failure: `stats.Failed` plus the `board_health`
sidecar, which cools the host off after three consecutive failures (`6h·2^(f-3)`, capped at
24h) and self-heals on success. A detail page that will not load skips that one posting
(`ok=false`), matching `catsone.detail`. One dead install never aborts the run.

The provider is **not** self-closing — it is absent from `sources.SelfClosingProviders`, so
withdrawn postings close through the 48-hour unseen-job sweep.

### Testing

Table-driven tests with builder functions producing fixture HTML inline, per the convention
in `catsone_test.go` (not `testdata/` files). Cases:

- stock XHTML template (boomit/crewlogix shape) and rewritten template (g4s shape);
- extra listing columns (indovision shape) — asserts no positional column assumptions;
- the "Apply here" pseudo-job is excluded;
- a detail page that 404s skips one job and keeps the rest;
- `<script>` inside a description is stripped by `sanitizeHTML`;
- prober unit test: `*.catsone.com` candidates are rejected.

## Out of scope

- Submitting applications through the portal — the adapter is read-only, as all adapters are.
- FreeATS — zero public installs; revisit only if that changes.
- Self-closing support.

## Risks

- **Small absolute volume.** 260 jobs is a fraction of a mid-size SaaS provider. The
  justification is the marginal cost, not the count: one adapter plus one prober, reusing
  every existing pipeline facility.
- **Template drift.** An install may rewrite the portal past recognition. Mitigation: parse
  routing invariants only; a broken install degrades to zero jobs and cools off rather than
  producing garbage.
- **Discovery ceiling.** urlscan only knows hosts someone has scanned. This finds the
  publicly visible tail, not every install — an accepted limit, recorded here so an empty
  future harvest is not misread as "no new installs exist".
