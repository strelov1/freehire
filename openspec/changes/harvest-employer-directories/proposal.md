# Harvest employers from a regional job-board directory

## Why

Search Console for August 2026 shows East African searchers converting an order of
magnitude better than the site average, against a catalogue that holds almost nothing
for them:

| Searcher country | Clicks | Impressions | CTR | Jobs in catalogue |
|---|---:|---:|---:|---:|
| Ethiopia | 20 | 211 | **9.5%** | 158 |
| Kenya | 28 | 420 | **6.7%** | 713 |
| Nigeria | 14 | 248 | 5.6% | 2,937 |
| Uganda | 14 | 291 | 4.8% | 119 |
| United States | 50 | 12,499 | 0.4% | 567,816 |

Site average CTR is 0.95%. The postings earning those clicks are development-sector
and regional-employer roles — World Vision Ethiopia, Greenlight Planet Uganda, Techno
Brain, Optimus Bank — all arriving through ATS platforms we already crawl.

**Three cheaper routes were measured and ruled out first.**

*Curating known employers is exhausted.* Of 28 well-known African tech employers, 19
are already in `sources/`. The 15 whose company pages resolve contribute **412 open
jobs between them**; the largest are World Vision (122), Greenlight Planet (88) and
Moniepoint (78). Adding more names of this kind buys tens of postings each.

*Regional aggregators are closed.* Fuzu returns 403 and MyJobMag serves a Cloudflare
challenge; BrighterMonday and Jobberman expose no sitemap and no `JobPosting` JSON-LD.
Only Ethiojobs is open, and it offers no API — treating it as a crawl source means a
bespoke HTML adapter for one country's ~929 postings.

*Guessing board slugs from employer names does not work.* Slugifying 25 names from the
Ethiojobs directory and probing them against the public Greenhouse and Workable board
APIs returned **0 hits**. The names are mostly international NGOs whose boards exist
under unrelated ids, and local firms with no ATS at all.

**What does work is using the directory as a worklist, not as a source.**
`ethiojobs.net/sitemap-companies.xml` lists **5,211 employers**, and each company page
carries the employer's own website in its `__NEXT_DATA__` payload. That is precisely
the `{name, website}` shape `cmd/harvest-ats resolve` already consumes, after which
`cmd/harvest-boards` validates every candidate board live against the platform's API
before anything is committed.

A 12-page sample found a website on 4 (33%), and the split is the informative part:
RTI International, ZOA, Farm Africa and Médecins Sans Frontières carry one; Bless Agri
Food Laboratory, Bridgetech PLC and Elilta Construction do not. The half of the
directory that is reachable is exactly the half that runs a real ATS.

## What Changes

- `cmd/harvest-ats` gains a third worklist source beside `extract` (collection
  datasets) and `universities` (the world-universities directory): a **directory**
  step that reads a regional job board's employer directory and emits the same
  `{name, website}` JSON, dropping employers already in the catalogue and those with
  no website.
- Ethiojobs is the first directory implemented. The step is written against a small
  directory interface so a second board is a new implementation, not a new command.
- Nothing downstream changes. `harvest-ats resolve` and `cmd/harvest-boards` run
  exactly as they do for the existing worklists, so every board committed is still
  live-validated against its platform's own API.

## Impact

- Affected specs: `domain-ats-harvest` (a new extraction requirement beside the
  collection-dataset one)
- Affected code: `cmd/harvest-ats/` — a new worklist source and its subcommand;
  `main.go` argument handling
- No schema change, no migration, no ingest-path change. The harvested boards land in
  `sources/*.yml` as ordinary entries.
- Committing boards is a reviewed diff, as it is today.

## Out of scope

- **Crawling Ethiojobs for postings.** The directory is a worklist here. Whether to
  build an adapter for the ~929 postings only Ethiojobs carries — the local employers
  with no ATS and no website — is a separate decision with a separate cost.
- **Yield promises.** The website rate (33%) is measured; the share of those websites
  that expose a detectable ATS board is not, and the first task measures it before the
  rest of the work is justified.
- **Widening the catalogue's scope.** These are employers whose postings the existing
  pipeline already accepts or rejects on its own rules. This change finds them; it
  changes no rule about what is kept.
