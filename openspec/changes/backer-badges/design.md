## Context

`internal/collections` is a fixed, code-owned registry of company tags. `Kind`
(`editorial` | `credential`) is its only render switch: `web/src/lib/credentials.ts`
filters `kind === 'credential'`, and everything else renders nothing anywhere. The
registry is mirrored to the frontend by `cmd/gen-contracts`, which emits `kind`
verbatim into `web/src/lib/generated/contracts.ts`.

Jobs already carry their company's tags (`internal/jobview/jobview.go:72`), and the
job feed already renders one badge family from them (`JobRow.svelte:291`). So the
data path for backer badges exists end to end; what is missing is a third kind, a
membership source for a16z, and the presentation.

a16z reaches us today only as `sources/speedrun.yml` — 11 companies. The file's own
comment explains why it is small: the network federates, and a portfolio company's
roles usually live on its own Ashby/Greenhouse/Lever board, which we crawl natively.
The fund's public directory is the real membership source.

Directory measured 2026-08-01 (`/api/v1/companies?source=freehire`): 876 companies,
16 560 open roles, `total_pages: 9`, three tiers — `a16z` 281, `speedrun` 36
(cohorts SR001–SR007, GF1), `market` 559.

## Goals / Non-Goals

**Goals:**
- A third collection kind whose members render as the backing brand's own logo.
- a16z membership covering the fund's real portfolio, not the ATS-less remainder.
- Badges on every surface where a company is named, plus the collection filter chips.

**Non-Goals:**
- Badges for the seven remaining editorial collections — no logo of their own.
- OG-image rendering. Committed SVGs would work with satori, but no OG surface was
  requested; `$lib/brands` is the seam if one is later.
- Fixing the logo-proxy monogram regression found while testing this change (see
  Risks) — real, but a separate change.
- Harvesting the directory's 559 `market`-tier companies as new ingest boards. They
  have open roles and domains, and that is a board-harvest task, not this one.

## Decisions

### A third Kind, not a boolean flag on editorial

`Kind` already carries exactly this meaning — "which group does this tag render
as" — and the spec says it "determines how a tag is presented". Adding
`KindBacker` extends the existing contract; adding `isBacker` alongside `kind`
would create a second, overlapping classifier whose disagreement with `kind` would
be undefined.

Consequence: `yc` and `techstars` change kind from editorial to backer. Nothing
reads `kind === 'editorial'` today, so no behaviour changes for them beyond gaining
a badge.

`cmd/gen-contracts` needs no change — it emits `c.Kind` as a string literal, so
`'backer'` reaches the generated union automatically.

### Two a16z collections, not one

`a16z-portfolio` (tier `a16z`) and `a16z-speedrun` (tier `speedrun`). Rejected
merging them into a single `a16z` tag: the directory itself distinguishes the two,
and OpenAI and a seed-stage SR005 company mean different things to a candidate
choosing where to apply.

### `market` tier is excluded, and the exclusion is a named test

The directory's largest tier is general market — TikTok, Walmart, Amazon, Procter &
Gamble, CVS Health. Importing it would put an a16z badge on Walmart: a false claim
about a real company, on a public page.

This is a one-line filter, which is exactly why it needs a test named for what it
prevents rather than a comment. A future reader adding a third tier, or a directory
that renames its tiers, must fail loudly rather than silently widen the tag.

### A fourth `Dataset` payload form

`Dataset` today resolves to a single body via `URL`, `Data`, or `ResolveURL`
(`collections.go:42-67`), enforced by `Valid()` as mutually exclusive. The directory
is paginated and ignores `page_size` (`?page_size=1000` returns 100; verified), so
no single URL expresses it.

Adding `Records func(context.Context, *http.Client) ([]Record, error)` as a fourth
mutually exclusive form keeps the "exactly one source" invariant and puts the
pagination where it belongs — in the source that knows it is paginated.

Alternatives rejected:
- **`ResolveURL` returning the first page.** Would silently import 100 of 876
  companies. The failure is invisible: a successful fetch that parses fine.
- **A `Pages func(int) string` field.** Encodes one pagination dialect into the
  registry; the next paginated source with a cursor would not fit.
- **Concatenating pages into one synthetic body for `Parse`.** Invents a payload
  format that no source publishes, to satisfy a signature.

Cost: the directory is fetched twice per run (once per tag), 18 requests total.
Acceptable for a daily worker; a shared cache is not worth its complexity until
a third tag reads the same source.

### Committed SVGs, not the logo proxy

`logo.freehire.me` resolves **by company name** (`logo.ts:6`). Tested against the
three brands: `Y Combinator` → correct, `Techstars` → correct, `a16z` → **a
screenshot of a Substack subscribe page**, `Andreessen Horowitz` → correct. An
abbreviation walked the favicon service onto an unrelated site.

A wrong brand mark is worse than no brand mark, and the proxy offers no way to
detect the miss. Committed SVGs also avoid a network request per feed card, render
crisply at any size, and keep a trademark display under review rather than under a
third party's control.

Explicitly **no monogram fallback**, unlike `CompanyLogo`. A letter tile where the
a16z mark belongs reads as a defect, not as graceful degradation.

### Badge placement: beside the company, not in the signal row

The job card's signal row already carries the reality chip, facet chips, credential
chips, and the country flag stack — four families, wrapping on a phone at the widths
job search actually happens at. A backer is a fact about the employer, so it goes
where the eye already reads the employer (`JobRow.svelte:246`), which also leaves
the signal row's role-level information intact.

### Filter chips take an optional icon on `FacetOption`

`ChipFacet` resolves its options from the `FACETS` registry by `param` and merges
live counts; `PillGroup` renders them. Adding an optional `icon` to `FacetOption`
lets any facet carry a mark, with no collections-specific branch in either
component.

## Risks / Trade-offs

- **[Short generic names slug-collide]** The directory carries names like `Dex`,
  `Sekai`, `Emanate`. Editorial collections match on `normalize.Slug` with no gate,
  and a wrong match would badge an unrelated company. → Not mitigated by a guessed
  gate. `cmd/import-collections -dry-run` resolves and reports exactly as a real
  run does; the report is read for both new tags and matches spot-checked by name
  before any write. If collisions appear, the directory's `location` field is the
  material for a `Gate`.
- **[`yc`/`techstars` membership disturbed by the kind change]** → The kind change
  touches presentation only; membership resolution is untouched, and the existing
  collapse guard aborts the run if either tag would lose most of its holders.
- **[The directory changes shape or disappears]** → A failed fetch aborts before any
  write (existing behaviour); a payload parsing to zero records is an error, not an
  empty membership. A partial paginated read is made an error for the same reason.
- **[Trademark display]** Three third-party marks rendered on a commercial site. →
  Nominative use: each names the company's actual backer, unaltered, without
  implying endorsement of freehire. Adding a fourth requires committing a file, so
  it stays a deliberate decision.
- **[Discovered, out of scope: the logo proxy's monogram fallback is dead]**
  `logo.dev`'s `fallback=404` no longer works on its *name* endpoint — an unknown
  name returns 200 with a photograph of a person, so the `@miss` → bodyless 404 →
  `<img onerror>` → SVG monogram chain in `provision/host2/nginx/logo` never fires.
  The same parameter still works on the *domain* endpoint (verified:
  `qqzz-fake-777-nonexistent.com` → clean 404), and `company_info.website` now
  supplies domains for part of the catalogue. → Recorded here; fixed separately.
  This change does not depend on the proxy at all, which is part of why committed
  SVGs are the right call.

## Migration Plan

1. Ship the registry, parser and import support; run `cmd/import-collections
   -dry-run` and read the report for `a16z-portfolio` and `a16z-speedrun`.
2. Run the import for real, then `make reindex` — `collections` is a search facet
   and the filter chips read its live distribution.
3. Ship the frontend. It degrades safely if run before the import: no company holds
   the new tags, so no badge renders.

Rollback: remove the two registry entries and re-run the import; the reconciler
drops tags it no longer manages. The kind change is presentation-only and reverts
with the frontend.

## Open Questions

None outstanding. The one genuine unknown — whether short directory names collide
on our catalogue — is answered by the dry-run report in step 1 of the migration
plan, before anything is written, rather than guessed at now.
