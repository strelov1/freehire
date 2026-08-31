# Harvest employers from a regional job-board directory

Date: 2026-08-31
Status: approved, not implemented

## What already exists

Do not rebuild any of this.

| Component | Location |
|---|---|
| The worklist shape | `cmd/harvest-ats/extract.go:14` — `companySite{name, website}`, the JSON both existing worklists emit and `resolve` consumes |
| Drop what we already have | `filterUnmatched(sites, existing)` (`extract.go:178`) |
| Drop duplicate sites | `dedupeByWebsite(sites)` (`extract.go:190`) |
| Existing worklist commands | `runExtract` (`main.go:98`, collection datasets), `runUniversities` (`main.go:68`) |
| Careers-page follow + ATS detect | `runResolve` (`main.go:137`) → per-provider `<provider>.seed.json` |
| Live validation and commit | `cmd/harvest-boards <provider> <seed.json>` |
| Paced concurrent page fetch | `resolveWorkers = 24`, `perPageTimeout = 20s` (`main.go`) |

The pipeline this change extends already ends in a live probe: `harvest-boards` keeps
a candidate only if the platform's own API reports open jobs for it, and rejects it
when the platform's reported company name disagrees with the seed's. Nothing this
change adds can put an unvalidated board into `sources/*.yml`.

## Scope

In scope: producing a `[]companySite` worklist from a job board's employer directory.

Out of scope: everything after that. `resolve` and `harvest-boards` are unchanged,
which is the point — the risky half of board discovery is already written and tested.

## Design

### A third worklist source, not a third pipeline

```
harvest-ats directory <company-slugs.txt>   # NEW → unmatched {name,website} JSON (stdout)
harvest-ats resolve   <unmatched.json>      # unchanged
harvest-boards <provider> <seed.json>       # unchanged
```

`runDirectory` mirrors `runExtract`: fetch, parse to `[]companySite`, then the same
`filterUnmatched` + `dedupeByWebsite` the other worklists use. The output is
byte-identical in shape, so `resolve` needs no knowledge that a directory exists.

### The one structural difference: a directory is paged

The existing worklists parse a single downloaded dataset. A directory is N pages —
5,211 for Ethiojobs — so `runDirectory` fetches a sitemap of company URLs and then
each page, under the same worker/timeout discipline `runResolve` already applies.

That is the only new I/O shape, and it is why the directory is behind an interface
rather than a function:

```go
// employerDirectory is a job board's list of the employers posting on it. The
// two methods split the one slow part (paging every company page) from the one
// site-specific part (reading a name and a website out of one page), so a second
// board is a parser, not a crawl loop.
type employerDirectory interface {
    // companyURLs lists the directory's company pages.
    companyURLs(ctx context.Context) ([]string, error)
    // parseCompany reads one company page into a site. A page carrying no
    // website yields ok=false — there is nothing for resolve to follow.
    parseCompany(html []byte) (companySite, bool)
}
```

### Ethiojobs

`companyURLs` reads `https://ethiojobs.net/sitemap-companies.xml`. Measured
2026-08-31: 2.2 MB, 5,211 `<loc>` entries, **68 seconds** to serve. The fetch needs a
timeout well above the client default, and the run is a one-off host tool, so slow is
acceptable — but it must not look like a hang, so the step logs its progress.

`parseCompany` reads the `website` field out of the page's `__NEXT_DATA__` script.
There is no `JobPosting` JSON-LD and no `<a href>` to the employer's site in the static
HTML — the website exists only in that payload.

**This is a scrape of a third party's internal payload and will break.** It is
tolerable here and nowhere near the ingest path for one reason: this is a run-once
host tool whose output a human reviews. When the shape changes, a run yields fewer
sites and someone fixes the parser; nothing in production notices. A `parseCompany`
that suddenly matches nothing SHALL make the run fail loudly rather than emit an empty
worklist that reads like "no new employers".

### What the directory will and will not give

A 12-page sample carried a website on 4. The split is not random: RTI International,
ZOA, Farm Africa and Médecins Sans Frontières have one; Bless Agri Food Laboratory,
Bridgetech PLC and Elilta Construction do not. The reachable half is the
internationally-operating half, which is also the half that runs an ATS — so the
website field is doing double duty as a relevance filter, not merely as an address.

The local half is unreachable by this route by construction, and the proposal says so
rather than implying the directory's 5,211 entries are 5,211 candidates.

## Components

| Unit | Responsibility | Depends on |
|---|---|---|
| `cmd/harvest-ats/directory.go` | The `employerDirectory` interface and the paged worklist run | the shared HTTP client, `filterUnmatched`, `dedupeByWebsite` |
| `cmd/harvest-ats/ethiojobs.go` | One board: its sitemap URL and its `__NEXT_DATA__` parser | — |
| `cmd/harvest-ats/main.go` | The `directory` subcommand | the above |

`parseCompany` is a pure function over bytes, so the parser is testable against a
saved page with no network — the same way `parseYCSites` and `parseUniversitySites`
are tested today.

## Testing

Unit, against saved fixtures:

- A company page carrying a website yields its name and website.
- A company page with an empty or absent `website` yields `ok=false`.
- A page whose `__NEXT_DATA__` is absent or unparseable yields `ok=false`, without
  panicking on a truncated payload.
- A sitemap fixture yields its company URLs, ignoring the image and asset `<loc>`
  entries that share the file.
- An employer already in the supplied slug set is dropped (`filterUnmatched`, already
  tested — the new test pins that the directory path runs it).
- Two directory entries resolving to one website collapse to one.
- A run where every page parses to `ok=false` exits non-zero rather than printing
  an empty list.

No test hits the live site.

## Risks

**The yield past the website rate is unmeasured.** 33% carry a website; what share of
those expose an ATS `resolve` can detect is unknown, and the analogues already in the
catalogue range from 7 to 122 postings each. The first task measures it on a sample
before the rest of the work is justified — if the detected-board rate is negligible,
this change should stop there.

**The parser depends on a third party's internal payload.** Mitigated by the loud
failure above, and bounded by being a host tool.

**One directory is one country.** Ethiojobs covers Ethiopia. Kenya, Nigeria and Uganda
carry more of the measured demand between them, and their boards are the ones behind
Cloudflare. The interface is what makes a second board cheap; this change does not
claim to have one.

**These employers post globally.** An international NGO's board carries its worldwide
vacancies, not only its East African ones. That is upside for the catalogue and a
reason not to describe the result as an East Africa feature.

## Related

- `openspec/specs/board-harvest/spec.md` — the live-validation discipline this feeds
- `openspec/specs/domain-ats-harvest/spec.md` — the requirement this sits beside
- `docs/seo-baseline.md` — the Search Console measurements the proposal cites
