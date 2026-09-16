## Context

See `proposal.md` for motivation. Three facts, verified against `internal/dict/location` and
prod data, shape the approach:

1. **The "global" fallback is not one thing.** `internal/dict/location/location.go:184-186`
   sets `regions=[global]` whenever `work_mode=="remote"` and nothing else resolved. But an
   explicit open-anywhere word (`anywhere`, `worldwide`, `international`, `global`, …) is
   *already* a first-class dictionary entry (`dictionaries.go:429-435`, resolved through the
   normal token loop via `nameToRegion`), so `"Remote - Anywhere"` reaches `regions=[global]`
   **without ever touching the fallback** — by the time the fallback check runs, `regions` is
   already non-empty. The fallback only ever fires for the genuinely signal-less case (a bare
   `"Remote"`, or a work-mode-only phrase like `"Work from home"`) — exactly the case the spec
   says should stay empty. Confirmed by tracing `location_test.go`'s own two affected cases
   (`"bare remote with no geography yields global"`, `"work-from-home marker with no place
   yields global"`) — **removing the fallback block changes only these two test cases**; every
   `"...Anywhere"/"...Worldwide"/"...International"` case is unaffected because it never
   depended on it.
2. **`title` is structurally excluded today.** `jobderive.Derive` passes `in.Title` to
   `classify.Parse` (seniority/category) and slug generation only; `location.Parse` and
   `location.EligibilityFromDescription` never see it.
3. **`EligibilityFromDescription` is a closed enumeration by design** (one phrase list per
   specific nationality — `usOnlyPhrases`, `ukOnlyPhrases`, …), tuned for precision with
   negation/sentence-boundary handling (`eligibility.go`). It cannot be extended to arbitrary
   "role is based in `<countries>`" prose by adding more fixed phrases — the phrase set is
   open-ended (any country/region name can follow the anchor). Reusing the *phrase* pattern
   would mean enumerating every country by hand a second time (drift risk noted in
   `AGENTS.md`'s "Company key" convention for a similar problem — one vocabulary, not several
   disagreeing copies).

`backfill-derive` does not reach already-stored rows for this class of bug: it feeds a job's
**currently stored** `regions`/`countries` back into `jobderive.Input.Regions`/`.Countries`
(`jobderive.go:152-161` treats a non-empty `Input.Regions`/`.Countries` as authoritative and
skips re-derivation entirely), so a stored `global` reproduces itself forever — the identical
trap `AGENTS.md` documents for `backfill-remote-perk-false-positive`.

## Goals / Non-Goals

**Goals:**
- Stop emitting `global` for a bare-remote posting with no actual open-anywhere/eligibility
  signal, without disturbing the genuinely-open cases (`Anywhere`, `Worldwide`, a stated
  `Global` marker).
- Read the job title for an ATS-convention restriction suffix, reusing the location
  dictionary's country/region resolution rather than a second copy of it.
- Recognize "role/based within `<country/region list>`" description prose as a restriction
  signal, generalized rather than enumerated per nationality.
- Reach already-stored jobs currently holding the incorrect `global` value.

**Non-Goals:**
- Replacing or restructuring `EligibilityFromDescription`'s existing citizenship-phrase rules
  — they stay as they are; the new description signal is additive, not a rewrite.
- Reading `offices[]`/`departments` from the Greenhouse adapter — confirmed empty of usable
  geography for the reported cases (see proposal's background research); a separate concern
  if pursued later, not part of this fix.
- Any change to the `regions=none` ("Not specified") search filter itself — it already exists
  (`remote-region-filters` capability) and needs no change to serve the jobs this fix moves
  out of `global`.

## Decisions

### 1. Delete the blanket fallback; do not add a new "open-anywhere marker list"

Because the open-anywhere words already resolve through the ordinary dictionary path
(`dictionaries.go:429-435`), the fix for the `global`-default bug is a straight deletion of
`location.go:184-186`, not a rewrite. No new marker set is needed — one already exists and
already works correctly for the cases it should cover.

**Alternative considered:** keep the fallback but gate it on an explicit marker list — rejected
as redundant work; the dictionary entries already are that marker list, just consulted through
the normal per-token resolution instead of a separate post-hoc check.

### 2. Title restriction extraction is anchor-gated, not a blind parse of the whole title

A title carries plenty of non-geographic bracketed/parenthetical text (`(React)`, `(Contract)`,
`(Full-time)`, level bands, …). Feeding the whole title through `location.Parse`'s tokenizer
risks a false-positive country match. Precision-first design (matching this package's existing
philosophy — see `eligibility.go`'s extensive negation/anchoring handling):

- Extract each bracketed (`[...]`) and parenthetical (`(...)`) substring from the title.
- Keep only a substring whose lowercase form contains one of a small anchor set: `remote`,
  `location`, `based`. This is deliberately narrow — it targets the observed ATS convention
  (`[Remote-US]`, `(Location - Australia or New Zealand)`, `(Based in the UK)`) rather than
  scanning arbitrary title text.
- Strip the anchor word and adjoining punctuation (`-`, `:`), then tokenize the remainder with
  the SAME separator normalization `location.Parse` already uses (comma/semicolon/slash/pipe/
  `" - "`/`" or "`) and resolve each token through the SAME `resolveGeoName`-style country/
  region lookup — no new dictionary, no new country-name list.
- A candidate substring that resolves nothing yields no geography and is silently dropped
  (never guesses) — consistent with every other rule in this package.

This step runs in `jobderive.Derive`, gated by the same `geoUnpinned(countries, regions)` check
that already gates the description-based eligibility override, and **before** the description
check (a title suffix is closer to an authored, structured statement than free prose, so it
takes precedence when both are present). A location that already resolved a country/region is
never consulted for title at all — the existing precedence chain (location → title → eligibility
description → region-scope description → explicit structured input) stays a strict fallback,
each link only filling what the previous left blank.

### 3. Description region-scoping is anchor-plus-tokenize, not a fixed phrase-per-country table

Same reasoning as (2), applied to description prose: anchor phrases (`role is based in`,
`located in`, `restricted to`, `open to candidates in`, `remote role within`, a small curated
set tuned against the reported examples) followed by tokenizing and resolving the trailing
clause against the location dictionaries, reusing the existing sentence-boundary/negation
scaffolding in `eligibility.go` (`phraseAsserted`, `sentenceAround`, `negatedSentence`) so a
denied statement ("not restricted to the US") still doesn't misfire. This is a new function
alongside `EligibilityFromDescription`, not a modification of it — the existing citizenship
rules keep their closed-enumeration shape because that shape is correct for them (a fixed,
short, unambiguous phrase per nationality); the new function's shape (anchor + dictionary
lookup) is correct for open-ended "list of countries" prose. `jobderive.Derive` calls both and
unions their results, same as it does today for the one existing function.

### 4. Backfill follows the `backfill-remote-perk-false-positive` pattern exactly

New one-off `cmd/backfill-remote-region-restriction`:
- Candidates from Meilisearch (`regions=global`), the same over-fetch-and-let-the-dictionary-
  decide shape `backfill-clearance`/`backfill-remote-perk-false-positive` already use — cheap,
  broad, and correct because the recompute step decides, not the candidate query.
- Recomputes geography from the row's stored `location`/`title`/`description` **alone**,
  calling the same derivation path `Derive` now uses — never feeding the row's own stored
  `regions`/`countries` back in as structured input (that is precisely the bug this backfill
  exists to undo).
- Writes only when the recompute no longer says bare `global` (`IS DISTINCT FROM`-guarded,
  idempotent, safe to stop and resume).
- `BACKFILL_REMOTE_REGION_MAX` caps one run, mirroring every other one-off backfill's knob
  convention (`worker.EnvInt64`).
- **Must be followed by a full `make reindex`**: `regions`/`countries` are not part of
  `content_hash`, so an incremental `search_outbox` push alone never reaches a pre-existing
  row — the same trap `backfill-clearance` documents in `AGENTS.md`.

**Alternative considered:** extend `backfill-derive` to special-case geography — rejected,
because `backfill-derive`'s whole design is "feed stored columns back in as structured input"
for the columns it doesn't own the derivation of; special-casing one column inside it makes the
worker's contract inconsistent (some columns re-derive from source, one re-derives from
nothing) rather than adding one small, single-purpose worker next to its three precedents.

## Risks / Trade-offs

- **[Recall loss]** A genuinely open-to-anywhere remote job whose location is a bare `Remote`
  and whose title/description states nothing further now lands in `regions=none` ("Not
  specified") instead of `global`, until/unless the employer's text says "Anywhere"/"Worldwide"/
  a region explicitly. → Mitigation: this is the exact, already-designed trade-off the
  `remote-region-filters` capability's `regions=none` chip exists for; it is not a gap this
  change introduces, it is the gap that capability was built to close. No further mitigation
  needed.
- **[Backfill scale]** The candidate query and recompute over however many stored jobs
  currently hold `regions=[global]` from a bare `Remote` location adds one more catalogue-wide
  pass. → Mitigation: bounded by `BACKFILL_REMOTE_REGION_MAX`, run in reviewed waves like
  `merge-companies`, not a single unbounded sweep.
- **[Anchor set is necessarily incomplete]** Both the title-anchor list (`remote`, `location`,
  `based`) and the description-anchor list are curated against the six reported cases, not
  exhaustive of every ATS's phrasing. → Mitigation: same precision-over-recall stance the rest
  of this package takes (missing a real restriction costs less than mislabeling an open role as
  restricted); the anchor lists are ordinary Go slices, easy to extend from the next report
  without a design change, the same way `eligibility.go`'s phrase lists already grew.
