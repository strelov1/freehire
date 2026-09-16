# Logo domain map — design

**Date:** 2026-09-16
**Status:** design approved, not implemented
**Repos:** `hire` (publisher) and `freehire-logo` (consumer)

## The problem

`https://freehire.me/jobs?company_slug=g2i` draws **two different logos for one
company**, and the one most rows get belongs to **a different business**.

Company logos are requested by the company's *name string*:
`web/src/lib/logo.ts:13` builds `https://logo.freehire.me/<name>`. That name comes
verbatim from whichever adapter ingested the row, and one employer arrives spelled
several ways:

| `jobs.company` | source | what `logo.freehire.me` answers |
|---|---|---|
| `g2i` | ashby (the company's own board) | 1814 B — "G2i Leadership Development", an unrelated business |
| `G2i` | himalayas, 4dayweek | same 1814 B, same wrong business |
| `G2i Inc.` | whatjobs-tr | 2984 B — the correct g2i.co mark |

All three rows carry `company_slug = 'g2i'`, so our *identity* rule is right. Only
the logo key is wrong: it is a raw adapter string, not the company.

The failure mode is not a missing logo — it is a **confidently wrong** one. The proxy
returns HTTP 200 with someone else's mark, so no consumer can detect it and fall back
to its monogram.

## What is already true

The proxy was built for this. `freehire-logo/internal/resolve/resolve.go` documents:

> The service never derives Domain itself: it has no company database, and a guessed
> domain is worse than none, since a wrong one returns a confident logo belonging to
> someone else, where no domain at all returns a monogram. **Whether to supply one is
> the caller's call.**

`internal/server/server.go:81` already reads `?domain=`, and it works today:

```
GET /g2i               → 1814 B  (wrong company)
GET /g2i?domain=g2i.co → 2984 B  (correct)
GET /G2i?domain=g2i.co → 2984 B  (correct, regardless of spelling)
```

We have never supplied a domain. We have one to supply:
`companies.company_info->>'website'` holds `https://g2i.co` for this company, and a
domain for **17,859** of the **239,879** companies that have jobs (7.4%).

## Why the fix does not go at the call sites

`companyLogoUrl` has ~25 call sites across `web/`, `extension/` and
`internal/application/mailtpl`. Roughly fifteen of them never see a company slug —
the experience bank (`ExperienceBankView.svelte`, a name typed into a CV), search
suggestions (`HeaderSearch.svelte`), community subjects (`SubjectHeader.svelte`),
the tailored-CV list (`CvList.svelte`). Threading a domain to each would fix about a
third of the surfaces and leave the rest guessing, which is not "always correct" — it
is a partial fix spread over twenty-five places.

Putting the knowledge behind the one HTTP call every surface already makes fixes all
of them at once and changes no call site.

## Design

**`hire` publishes a map; `freehire-logo` consults it.**

1. A new `cmd/publish-logo-domains` worker writes a snapshot file: **normalized
   company name → bare registrable domain**, one entry per *observed spelling* of
   every company whose `company_info` records a website.

   ```json
   {
     "version": 1,
     "normalization": "lower-collapse-ws",
     "generated_at": "2026-09-16T12:00:00Z",
     "entries": { "g2i": "g2i.co", "g2i inc.": "g2i.co" }
   }
   ```

2. The proxy loads that file. When a request carries no `?domain=`, it looks the
   name up and uses the domain it finds. No hit, no file, no env var → today's
   behaviour, byte for byte.

`hire` keeps every piece of domain knowledge: which spellings are one company — the
stored `jobs.company_slug`, which `normalize.CompanySlug` computed at ingest — and
what that company's website is. The proxy gains a lookup table, not a rule.

### Why the map is keyed by name and not by slug

Most call sites only have a name. Keying by the name the caller already sends is what
makes the fix reach all twenty-five of them without touching any.

### Normalization is a cross-repo contract

The worker must key entries exactly as `freehire-logo`'s `store.Normalize` does:
lower-case, runs of whitespace collapsed to one, ends trimmed, **punctuation left
alone**. Two copies of one rule in two repositories drift silently, and the symptom
of drift here is "the map exists and matches nothing".

Mitigation: the file declares its `normalization` value, and the proxy **refuses a
file whose value it does not implement**, logging and continuing without a map. A
refused map is today's behaviour; a silently mismatched one is a map that does
nothing while looking healthy.

### Colliding names are dropped, never resolved

If two company slugs with *different* domains normalize onto one name, the entry is
omitted and counted. Picking either one returns a confident logo belonging to the
other company — the exact failure this change exists to remove. A monogram or a name
guess is the better answer.

## What was cut, and the evidence for cutting it

The approved sketch had a second half: publish a **canonical name** per company too,
so a company with several spellings and *no* known domain would at least show one
consistent logo everywhere. 12,923 companies with open jobs have more than one
spelling.

It is cut, because it can make a correct logo wrong. The canonical name would come
from `companies.name`, and for g2i that value is `"g2i"` — the spelling that resolves
to the **wrong** business. Collapsing g2i's three spellings onto it would have made
every row uniformly wrong, replacing a visible inconsistency with an invisible error.
Without a domain there is no evidence for choosing between spellings, so the change
makes no such choice.

Consistency still arrives for every company the map covers: all of g2i's spellings
map to `g2i.co`, so all of them render one correct logo.

## The query, and a measurement that reversed the obvious choice

The worker needs every distinct `(company_slug, company)` pair among open jobs.
Measured on production, 2026-09-16:

| Query | Rows | Time |
|---|---|---|
| `SELECT DISTINCT company_slug, company FROM jobs WHERE closed_at IS NULL` | 412,648 | **53 s** (seq scan) |
| the same, joined to the 17,859 companies that have a website | subset | **200 s** (index nested loop) |

Restricting to the companies we care about is **four times slower**. The narrow query
drives 17,859 index searches, each fetching ~102 heap rows at random: 1.59M blocks of
random I/O against the seq scan's sequential read of the same heap. Narrowing the row
count widened the I/O.

So the worker runs the **full scan once**, and filters its result in Go against a
second, cheap query — the 17,859 companies whose `company_info` records a website,
read as one `slug → website` set before the scan begins. This also leaves the door
open to a future pass that needs the other spellings, without a second scan.

53 s of sequential read once a day is acceptable on a host whose bottleneck is the
crawl fleet, but it is the dominant cost of this change and belongs in the daily
schedule deliberately, not beside another heavy pass.

## Components

**`hire`**

- `cmd/publish-logo-domains` — `worker.Main`/`Bootstrap`, `DATABASE_URL` only.
  Writes to a temp file and renames, so the proxy never reads a half-written map.
  A run that would publish an **empty** map refuses to swap: a catalogue yielding
  nothing is a failed measurement, not an empty catalogue (the rule
  `cmd/build-suggestions` already follows).
  `LOGO_DOMAIN_MAP_OUT` unset → a no-op that never opens the pool, which is also
  how the change ships dark and how it is rolled back.
- `internal/job/logodomain` — extracting a bare registrable domain from a stored
  `website` value, the name normalization, and the collision rule. Pure, table-tested.
- A daily systemd unit and timer under `deploy/systemd/`.

**`freehire-logo`**

- `internal/domainmap` — loads and periodically reloads the file (mtime check),
  validates `version` and `normalization`, exposes `Lookup(normalizedName) string`.
- `internal/server` — one branch: when `?domain=` is absent, consult the map.
  An explicit `?domain=` from a caller still wins, unchanged.
- `LOGO_DOMAIN_MAP` unset → the map is never loaded and the handler is unchanged.

## Cache behaviour

No cache busting is needed. `server.go:249` already folds the domain into the cache
key — "the same name asked with and without one can legitimately resolve to different
companies". A name that gains a domain therefore gets a **new** key, and the stale
wrong rendition simply goes cold rather than being served.

## Rollback

Unset `LOGO_DOMAIN_MAP` on the proxy, or delete the file. Both restore today's
behaviour exactly. The `hire` side is inert without `LOGO_DOMAIN_MAP_OUT`.

## Deployment order

The publisher must run before the consumer is pointed at anything: a proxy configured
with a path that does not exist yet logs a missing map on every reload. Ship
`hire`'s worker, let one run produce a file, then deploy the proxy.

`freehire-logo` is **not** carried by `release.sh` — its binary is copied to the host
by hand (`deploy/AGENTS.md`). The change is only half delivered until that copy
happens, and a half-delivered change looks exactly like a working one.

## Testing

- `internal/job/logodomain`: website values that are URLs, bare hosts, `www.`-prefixed,
  junk, and empty; names differing only in case and whitespace collapsing onto one key;
  two slugs with different domains colliding on a name being dropped.
- `cmd/publish-logo-domains`: an integration test over a seeded database asserting a
  company with a website and three spellings yields three entries with one domain, and
  that an empty result refuses to swap.
- `freehire-logo/internal/domainmap`: a file with an unknown `normalization` is
  refused; a missing file is not an error; a rewritten file is picked up on reload.
- `freehire-logo/internal/server`: an explicit `?domain=` beats the map; a mapped name
  reaches the resolver with the mapped domain; an unmapped name is unchanged.
- End to end against production data, by hand: `GET /g2i` returns the 2984 B rendition.

## Out of scope

- **Growing domain coverage past 7.4 %.** `cmd/backfill-company-info-wikipedia`
  already fills `company_info` daily, and every company it resolves joins the map for
  free. Mining domains from apply URLs or posting bodies is a separate change.
- **A curated `slug → domain` override.** Nothing needs one yet — g2i is fixed by the
  data we already store. The seam is `internal/job/logodomain`, where a curated map
  would be consulted ahead of the stored website.
- **The `is_tech`/category gate that hides 21,636 open ashby postings from search.**
  Found in the same investigation, unrelated cause, its own change.
