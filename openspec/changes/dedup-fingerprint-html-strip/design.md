## Context

`role_fingerprint` (`internal/jobhash/rolefingerprint.go`) is the repost-identity key: `sha256(company_slug ⋮ normalizeRoleText(stripTrailingClause(title)) ⋮ normalizeRoleText(description))`. `normalizeRoleText` today is `strings.Join(strings.Fields(strings.ToLower(s)), " ")` — lowercase + whitespace fold only. Job descriptions are stored as **sanitized HTML** (an ingest-time `bluemonday` prose allowlist in `internal/sources/sanitize.go`), so the hashed text still contains tags (`<p>`, `<br>`, `<li>`, …) and entities (`&amp;`, `&#39;`). Any markup difference between two otherwise-identical postings produces different fingerprints and a duplicate card.

Prod measurement (2026-07-21): within `(company, title)` groups that already carry >1 fingerprint, stripping tags before hashing collapses **~29k fingerprints across ~18k groups** — postings whose only difference is markup. (The measurement stripped tags only, no entity decode, so it is a conservative floor.) The Towa "Senior Fullstack Engineer" case that motivated this is NOT one of these: Bregenz vs Wien differ by a real visible sentence (an Austrian Kollektivvertrag clause), so they legitimately stay separate under exact-match — fuzzy matching for that class is deferred.

## Goals / Non-Goals

**Goals:**
- Make `role_fingerprint` compare the **visible text** of title and description, not their markup, so markup-only duplicates collapse.
- Reuse the project's existing sanitizer stack; add no new dependency and no new heuristic threshold.
- Keep the change to a single pure function (`normalizeRoleText`) so `UpsertJob`, `cmd/ingest`, and `cmd/backfill-role-fingerprint` all pick it up unchanged.

**Non-Goals:**
- No fuzzy / Jaccard / similarity-threshold matching — the fingerprint stays an exact-match on normalized visible text (separate future change).
- No change to `stripTrailingClause` (title city-suffix), the two-word guard, the field delimiter, or the geography-union reindex behavior.
- No re-sanitizing or rewriting of the stored `description` column — normalization happens only inside the fingerprint computation.

## Decisions

**1. Normalize to visible text inside `normalizeRoleText`, applied to both fields.**
`normalizeRoleText` already runs on both the stripped title and the description, so adding the HTML→text step there keeps one code path and one contract. Titles are usually plain, but running the same step folds entity-encoded titles (`R&amp;D` → `r&d`) consistently and costs nothing measurable (fingerprints are computed at ingest, not on a hot path). Alternative — description-only — was rejected as an asymmetric special case with no benefit.

**2. Strip tags with a `<[^>]*>` regex replaced by a SPACE, then decode entities with `html.UnescapeString`.**
The tag strip replaces each tag with a space (not the empty string) so words separated only by a block element keep their boundary; entity decoding then converts `&amp;`→`&`, `&#39;`→`'`, and `&nbsp;`→U+00A0 (which `strings.Fields` folds to a space). Order: **strip tags first on the raw HTML, then unescape** — so an escaped angle bracket in visible text (`a &lt; b`) is decoded to a literal only after real tags are gone and is never mistaken for a tag. This matches the prod measurement exactly (which inserted spaces at tag positions and yielded ~29k), so the shipped collapse equals the measured one.
- _Alternative — `bluemonday.StrictPolicy().Sanitize()`:_ the project's existing text-extraction idiom (`internal/sources/gupy.go`), but empirically it **deletes** tags without inserting a boundary, gluing words across blocks (`word1<br>word2`→`word1word2`, `<li>a</li><li>b</li>`→`ab`). That both diverges from the measurement and would *fail* to collapse the very markup-only dupes this change targets (a `<br>`-separated posting would not match its space-separated twin). Rejected.
- _Alternative — `html2text.FromString`:_ preserves boundaries but adds layout opinions (table formatting, inline link URLs) that widen the hash surface for no dedup benefit. Rejected; the regex is a pure, boundary-preserving text projection.
- _Over-strip safety:_ a greedy `<…>` could in principle eat visible text between a stray unescaped `<` and a later `>`. This vector does not exist here: descriptions are **already** sanitized at ingest (`internal/sources.SanitizeHTML`, a bluemonday prose allowlist), so stored HTML is well-formed and any `<`/`>` in visible text is entity-escaped. Titles are plain text and do not contain `<…>`.

**3. The tag regex is compiled once at package scope.**
`var htmlTag = regexp.MustCompile("<[^>]*>")` — like the existing `trailingClause` regex — so no per-hash recompilation. No new dependency is added (`html` is stdlib; `bluemonday` remains only the ingest-time sanitizer).

**4. The description stays in the key (over-merge guard unchanged).**
Two postings collapse only when both the stripped title AND the visible description match exactly. Normalization narrows what "match" means (visible text, not markup) but does not relax exactness — postings with any visible-text difference still diverge.

## Risks / Trade-offs

- **[Over-merge from too-aggressive stripping]** → the tag regex only strips well-formed tags from already-sanitized HTML (no stray `<`/`>` in visible text) and merges still require byte-identical visible text, which is a genuine duplicate. No similarity fuzz is introduced.
- **[Inline mid-word tag fails to merge]** → replacing every tag with a space means a word split by an inline tag (`wo<b>rd</b>` → `wo rd`) will not collapse with a plain `word` twin. This is the flip side of space-insertion (which correctly fixes the dominant block-separated case); it only ever causes a failure-to-merge, never an over-merge, so it stays on the conservative side. Mid-word inline tags are rare in prose descriptions and not represented in the measured ~29k block-separated dupes.
- **[Stale fingerprints until backfill runs]** → the code change only affects rows written after deploy; existing rows keep old fingerprints and would show a transient split between old and re-crawled rows. Mitigation: run `cmd/backfill-role-fingerprint` immediately after deploy, before the next reindex, so the whole table is recomputed in one shot.
- **[Reindex contention]** → the follow-up `make reindex` must not stack with the semantic or companies reindex (shared flock rules). Mitigation: run in a low-traffic window on its own flock, per existing reindex ops.
- **[Normalization drift on a future change]** → fingerprints are internally consistent (all rows rehashed by the same code) and reconciled by the backfill, so any future change to the strip just triggers another backfill; there is no external contract on the hash value.

## Migration Plan

1. Merge + deploy the code change (new `normalizeRoleText`). New/re-crawled rows immediately hash on visible text.
2. Run `cmd/backfill-role-fingerprint` (tune `BACKFILL_CONCURRENCY`) to recompute every row; the `UpdateJobRoleFingerprint` guard writes only moved rows, so it is idempotent and re-runnable.
3. Run `make reindex` (own flock, low-traffic window) — its `duplicate_of` recompute collapses the newly-clustered reposts and unions their geography onto each canon.
4. Spot-check the Towa cluster and a sampled set of the ~18k affected groups: markup-only variants collapsed, visible-text-distinct postings (Krakau PLN; Bregenz/Wien KV clause) stayed separate.

**Rollback:** revert the code and re-run `cmd/backfill-role-fingerprint` + reindex; the fingerprint returns to its previous value deterministically (no data loss — only the derived `role_fingerprint` column and search index change).

## Open Questions

None blocking. Fuzzy near-duplicate matching (the Bregenz/Wien class) is tracked as a separate future change.
