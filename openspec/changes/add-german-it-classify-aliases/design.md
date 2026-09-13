## Context

`internal/dict/classify/dictionaries.go` resolves `jobs.category` from a
title via `matchOrdered`, which walks a priority-ordered `[]aliasEntry` table
and returns the category of the FIRST alias found as a whole word
(`wordmatch.Contains(title, alias, wordmatch.UnicodeBoundary)`). A hyphen or
space is a valid boundary character on both sides, so a bare compound alias
already reaches a hyphen- or space-prefixed spelling of the same word (the
doctrine the Russian `разработчик`/`программист` entries and the existing
German `entwickler` cluster both rely on) — it does NOT reach a spelling
where the compound itself is fused differently (`softwareentwickler` vs
`software-entwickler` vs `software entwickler` are three distinct substrings
to `strings.Index`, hence three aliases). See proposal.md - Why for the
`CategoryUnresolved` mechanism this feeds.

The affected titles come overwhelmingly from one source, `arbeitsagentur`
(confirmed via a local DB check), which is expected: it is the German
Federal Employment Agency board.

## Goals / Non-Goals

**Goals:**
- Add the missing German fused-compound aliases so the affected titles
  resolve a `category`, which is both what `CategoryUnresolved` reads and
  what feeds `is_tech` derivation (tech-classification's category-based
  rule) — no separate `is_tech`/tech.go change needed.
- Keep every new alias evidenced against real titles pulled from the local
  DB (see proposal.md), not guessed — consistent with "Dictionaries: ...
  never guess, emit nothing for unknowns" (AGENTS.md).

**Non-Goals:**
- Splitting `Fachinformatiker` into every possible qualifier
  (`Daten- und Prozessanalyse`, `Digitale Vernetzung`, support-flavored
  postings, etc.) — the two dominant, unambiguous tracks
  (Systemintegration/Anwendungsentwicklung) get their own category; every
  other phrasing falls through to the bare alias's `devops` default. This
  trades some category precision for search visibility, the same trade-off
  `it-title-coverage`'s own "IT Specialist"/"IT Technician" → `support`
  entry already makes for an ambiguous-track generic IT title.
- Retroactively reclassifying existing rows. A resolved dictionary entry
  only changes what a title resolves to going forward (ingest) and on the
  next `backfill-derive` pass; running that backfill and the follow-up
  `make reindex` is a deploy-time operational step (see proposal.md -
  Impact), not part of this change's tasks.
- Touching `internal/dict/classify/tech.go` (the separate `is_tech`-only
  detector) — unnecessary, since a resolved category already flips
  `is_tech` true through the existing tri-state derivation rule.

## Decisions

- **Bare alias for the six unambiguous terms** (`systemadministrator`,
  `netzwerkadministrator`, `datenbankadministrator`, `netzwerktechniker`,
  `softwaretester`, `anwendungsentwickler`): every real title sampled from
  the local DB for these words was IT work, with no cross-domain lookalike
  the way "Systems Engineer" or "Systemtechniker" have — so a bare alias is
  safe and matches the existing convention for other unambiguous bare
  German/Russian tokens in the file.
  - Alternative considered: qualify all of them with `IT`/`system` prefixes
    for symmetry with `Systemtechniker`/`Systemelektroniker`. Rejected — it
    would silently exclude the many sampled titles that carry no such
    prefix (e.g. bare "Datenbankadministrator (m/w/d)"), for a risk that
    real data does not show.
- **Qualified-only alias for `Systemtechniker`/`Systemelektroniker`**: the DB
  sample includes non-IT usages ("Systemtechniker Elektrotechnik",
  "Systemtechniker Sicherheitstechnik"), so only the `IT`-qualified spellings
  (hyphen and space forms, each its own alias string) resolve; the bare word
  stays unresolved, mirroring the existing "Systems Engineer" family's
  blind-non-IT-lookalikes doctrine in `it-title-coverage`.
- **`Fachinformatiker`: qualified phrases declared before the bare
  fallback**, following the file's existing "more specific alias first"
  ordering rule (same as the `1С`/`аналитик 1С` precedent). The qualified
  phrase only matches when the qualifier is directly adjacent (no
  intervening "für"/"/in"/"m/w/d"); titles where it isn't adjacent fall
  through to the bare alias, categorized `devops` (the numerically dominant
  track in the sample).
- **`SPS-Programmierer` excluded**: it is already a deliberate,
  documented exclusion (industrial/PLC programming, same as "CNC
  Programmer") — adding it would contradict existing, evidenced intent.
- **Placement in `dictionaries.go`**: directly after the Russian bare-token
  cluster (`программист`/`разработчик`) and before "The Russian engineering
  family" section — the same reasoning that cluster's own comment gives
  (bare tokens reaching every hyphen/space-prefixed spelling) applies to
  these German fused compounds too, so the new block sits next to its
  closest doctrinal precedent rather than the English
  `administrator`/`technician`/`tester` entries scattered earlier in the
  file. A short comment records the boundary-matching rationale (bare vs.
  qualified-only) for future editors, matching the file's own documentation
  density elsewhere.

## Risks / Trade-offs

- [Risk] A German title using `Fachinformatiker`/`Systemadministrator`/etc.
  in a sense this design didn't sample could be mis-tagged. → Mitigation:
  every new bare alias was checked against the full distinct-title list for
  that word in the local DB (not just a top-N sample) before being accepted
  as unambiguous; the two words that showed real ambiguity were restricted
  to qualified-only aliases instead of being added bare.
- [Risk] The Fachinformatiker bare fallback assigns `devops` to some
  Anwendungsentwicklung-track postings whose qualifier isn't adjacent to the
  word (e.g. "Fachinformatiker (m/w/d) für Anwendungsentwicklung" — "für"
  breaks the two-word phrase match). → Accepted trade-off: the posting is
  still resolved and searchable (this change's actual goal), just under a
  coarser category than ideal; documented as a Non-Goal above rather than
  chased with a combinatorial set of "für"-inclusive phrase variants.
- [Risk] Existing open postings ingested before this change carry a stale
  empty `category`. → Not this change's fix; documented as a required
  follow-up operational step in proposal.md - Impact.

## Migration Plan

No schema or data migration. Deploy is a normal code release; the follow-up
`cmd/backfill-derive` + `make reindex` operational pass (documented in
proposal.md, tracked outside this change's tasks) is what reaches existing
rows.
