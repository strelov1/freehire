# Backer badges — accelerator and fund logos on the job card

Date: 2026-08-01

## Problem

A company's curated collections already reach the frontend on every job
(`internal/jobview/jobview.go:72`), but only the *credential* subset renders
(`JobRow.svelte:291` → `CredentialBadge.svelte`). The editorial tags — including
`yc` and `techstars` — are invisible everywhere except the `/collections` hub.

"This company was picked by Y Combinator" is one of the strongest trust signals a
job card can carry, and today a reader gets it only by recognising the company
name. Meanwhile a16z, the third brand worth showing, is not a collection at all:
it reaches us as `source='speedrun'` on 11 companies in `sources/speedrun.yml`,
while the fund's actual directory lists 317 relevant ones.

## What ships

A **backer badge**: the accelerator's or fund's own logo, rendered next to the
company name, for three brands — Y Combinator, Techstars, a16z.

Deliberately *not* shipping: badges for the remaining editorial collections
(`bigtech`, `unicorn`, `fortune500`, `mag7`, `ai`, `ai-native`, `european`,
`eastern-roots`). They have no logo of their own, so each would need an invented
icon, and three or four chips on one card is noise. They stay filters.

## Design

### 1. A third Kind in the registry

`internal/collections` carries `Kind` (`editorial` | `credential`), and it is the
only render switch there is: `credentials.ts:46` filters `kind === 'credential'`
and everything else silently renders nothing.

Add `KindBacker = "backer"`. Move `yc` and `techstars` onto it, and add two new
tags. `cmd/gen-contracts/vocab.go:111` emits `kind` verbatim, so the new value
reaches `web/src/lib/generated/contracts.ts` with **no generator change**.

| slug | title | Kind | membership |
|---|---|---|---|
| `yc` | Y Combinator | backer | unchanged (existing dataset) |
| `techstars` | Techstars | backer | unchanged (existing dataset) |
| `a16z-portfolio` | a16z | backer | **new** — Speedrun directory, `tier=a16z` |
| `a16z-speedrun` | a16z Speedrun | backer | **new** — Speedrun directory, `tier=speedrun` |

Two a16z tags, not one: "in the fund's portfolio" (OpenAI, Anduril) and "went
through the accelerator" (Aghanim, Dex) are different facts of different
strength, and collapsing them would tell a candidate that a seed-stage SR005
company and OpenAI carry the same signal.

### 2. The a16z membership source

`https://speedrun-talent-network.com/api/v1/companies?source=freehire`, the same
public API `internal/sources/speedrun.go` already crawls. Measured 2026-08-01:
876 companies, 16 560 open roles, three tiers.

| `tier` | count | meaning |
|---|---|---|
| `a16z` | 281 | the fund's portfolio |
| `speedrun` | 36 | accelerator cohorts (`cohort`: SR001–SR007, GF1) |
| `market` | 559 | **general market — no a16z relationship at all** |

**The `market` tier must never be imported.** It contains TikTok, Walmart,
Amazon, Procter & Gamble and CVS Health. A naive whole-directory import would put
an a16z badge on Walmart. This is the single most dangerous mistake this change
can make, and it is a one-line filter — which is exactly why it needs to be
stated, tested, and not left to be re-derived later.

Two API constraints shape the fetch:

- **`page_size` is ignored.** `?page_size=1000` returns 100 anyway; the response
  reports `total_pages: 9`. Pagination is mandatory.
- **The directory is one payload for two tags.** Each tag filters the same fetch
  by `tier`, so the run costs 18 requests rather than 9. Acceptable for a daily
  worker; not worth a shared cache until it isn't.

`Dataset` today declares exactly one of `URL` / `Data` / `ResolveURL`
(`collections.go:42-67`), all of which resolve to a single body. Add a fourth,
mutually exclusive form:

```go
// Records fetches and parses the membership itself, for a source that no single
// URL can express — a paginated directory, say. Mutually exclusive with the
// other three, enforced by Valid().
Records func(context.Context, *http.Client) ([]Record, error)
```

`internal/collections/speedrun.go` implements it: walk `total_pages`, keep the
requested tier, return one `Record` per company name.

### 3. Matching risk, and how it gets answered

Editorial collections match on `normalize.Slug` with no `Gate`. That is fine for
`Fortune 500`, and risky here: the Speedrun directory carries short, generic
names — `Dex`, `Sekai`, `Emanate` — that could slug-collide with unrelated
companies in a 2.5M-job catalogue.

This is not resolved by guessing a gate. `cmd/import-collections -dry-run`
resolves, matches and reports exactly as a real run does, and the change is not
done until that report has been read for both new tags and the matches spot-
checked by name. If collisions show up, the directory's `location` field is the
material for a `Gate`; adding one before seeing the report would be inventing a
rule for a problem that may not exist.

The existing collapse guard already protects `yc`/`techstars` from a bad run.

### 4. The logos

Three inline SVGs committed to `web/src/lib/brands/` (`YCombinator.svelte`,
`Techstars.svelte`, `A16z.svelte`), each a bare `<svg>` accepting a `class`.

Not the logo proxy, though it was the obvious candidate and was tested first.
`logo.freehire.me` resolves **by name** (`logo.ts:6`), and on these three brands
that mis-resolves in a way no fallback catches:

| key | result |
|---|---|
| `Y Combinator` | correct orange Y |
| `Techstars` | correct wordmark |
| `a16z` | **a screenshot of a Substack subscribe page** |
| `Andreessen Horowitz` | correct maroon a16z mark |

An abbreviation walked the favicon service onto an unrelated site. Committed SVGs
also mean no network request per card in a 20-row feed, crisp rendering at any
size, and no dependency on a third-party service inside a *brand* signal.

Adding a fourth brand later means committing one more file — a deliberate,
verifiable step, which is the right cost for a trademark being displayed.

### 5. Presentation

`web/src/lib/backers.ts`, mirroring `credentials.ts`: resolve a company's or
job's `collections` to badges, in registry order, with unknown slugs yielding
nothing. Copy per slug lives here (tooltip text); the icon component is looked up
by slug.

`web/src/lib/components/BackerBadge.svelte` renders the logo at `size-4`, with an
accessible name (`Backed by Y Combinator`) and a `title`. Following
`CredentialBadge`'s precedent, the accessible name carries the full sentence
rather than relying on hover, which does not exist on the phones most job search
happens on.

**No monogram fallback.** `CompanyLogo` degrades a missing logo to a coloured
initial; for a backer badge that would render a letter "A" where the a16z mark
belongs, which reads as a bug. The SVG is committed, so it cannot be missing —
and an unknown slug renders nothing at all.

Placement per surface:

| surface | placement |
|---|---|
| `JobRow.svelte` | logo only, right of the company name (`:246`) |
| job page `/jobs/[slug]` | logo + text, links to the collection landing |
| `CompanyHeader.svelte` | beside the existing `CredentialBadge` |
| `/companies` cards | logo only, right of the name |
| `/collections` hub + landing header | logo beside the title |
| filter chips | logo inside the chip, left of the label |

The feed's placement is the one design decision worth defending: the signal row
already carries a reality chip, facet chips, credential chips and a country-flag
stack, and a fourth chip type would push it to a second line on a phone. The
badge is a fact about the *employer*, so it belongs where the eye already reads
the employer.

Filter chips take an optional `icon` on `FacetOption` (`$lib/facets`), consumed
by `PillGroup.svelte`. One optional field, no special case for collections.

## Out of scope

- **OG images.** Committed SVGs would work with satori, but no OG surface was
  requested; the seam is the shared `$lib/brands` module.
- **The logo-proxy monogram regression.** Discovered while testing this change
  and worth recording, but a separate fix: `logo.dev`'s `fallback=404` no longer
  works on its *name* endpoint — an unknown name returns HTTP 200 with a
  photograph of a person rather than a 404, so the `@miss` → bodyless 404 →
  `<img onerror>` → SVG monogram chain in `provision/host2/nginx/logo` is dead.
  The same parameter still works correctly on the *domain* endpoint (verified:
  `qqzz-fake-777-nonexistent.com` → clean 404), and `company_info.website` now
  gives us domains for part of the catalogue — so the fix exists, it is just not
  this change.
- Badges for the seven remaining editorial collections.

## Verification

- `internal/collections` registry test covers the new kind and both new tags.
- A test asserts `market`-tier records are dropped, named for what it protects
  against (a16z badge on Walmart).
- `cmd/import-collections -dry-run` read for both new tags before any write;
  matched names spot-checked.
- `make reindex` after the write — `collections` is a search facet, and the
  filter chips read the live distribution.
- Visual check of the feed at <500px width, per the project's headless-Chrome
  note that `--window-size` lies below that threshold.
