## Context

`internal/skilltag.Parse` resolves a curated technology vocabulary from free
text in two passes (`skilltag.go`):
1. **Phrase pass** — for each `phraseAliases` entry (multi-word/punctuated, e.g.
   `distributed systems`, `c++`, `node.js`), `wordmatch.Contains(norm, alias,
   ASCIIBoundary)` tests for a whole-token occurrence.
2. **Word pass** — `wordTokenRE = [a-z0-9]+` splits the normalized text into bare
   tokens, each looked up in the `wordAliases` map.

`normalize()` strips HTML, lowercases, trims. It does **not** touch `-`/`_`, and
`wordTokenRE` splits on them. `ASCIIBoundary` additionally guards a leading `.`
or `-` as an invalid left boundary (so `asp.net`↛`.net`, `objective-c`↛`c`).

This is a curated dictionary that feeds the production `jobs.skills` facet — the
project's invariant is "resolve known vocabulary, never guess." The fixes must
preserve that; embeddings/fuzzy matching are explicitly out.

## Goals / Non-Goals

**Goals:**
- `distributed-systems` / `distributed_systems` resolve exactly like
  `distributed systems`.
- Curated uppercase acronyms (`RAG`, `ETL`, `ELT`, `gRPC`, …) resolve while their
  ambiguous lowercase forms stay unresolved.
- Zero regressions: punctuated canonicals (`c++`, `node.js`, `asp.net`, `c#`)
  still match; no new false positives; dedup+sort invariant intact.

**Non-Goals:**
- Embeddings, semantic similarity, or any fuzzy/probabilistic matching.
- Growing the general vocabulary (that is the separate LLM-mining track).
- Changing consumers (`jobderive`, `ExtractResumeSkills`) or the facet schema.

## Decisions

### D1 — Collapse `-`/`_` to space on BOTH text and phrase aliases

**Do NOT collapse the text.** The first attempt — collapse `-`/`_`/whitespace to a
space in both text and aliases — is clean for matching but breaks the boundary
guard (see D2), so it was abandoned during implementation. Instead, leave the
lowercased text with its separators intact and make the **phrase matcher**
separator-insensitive per alias:

- A multi-word alias (split on `[-_\s]+` into segments) compiles once at init to a
  regex joining its `regexp.QuoteMeta`'d segments with `[-_\s]+`, e.g. `distributed
  systems` → `distributed[-_\s]+systems`. It matches `distributed-systems`,
  `distributed_systems`, `distributed systems`, and mixed forms
  (`retrieval-augmented generation`) alike.
- A single-token alias (no `-`/`_`/space: `c++`, `node.js`, `ci/cd`) keeps the
  cheaper `wordmatch.Contains` substring path unchanged.
- Each regex hit is boundary-checked with the SAME `wordmatch.ASCIIBoundary` used
  by the substring path (applied to the match `[start,end)`), so the boundary
  semantics are identical to before.

*Preserve `.`/`#`/`+`/`/`:* `sepRE = [-_\s]+` excludes them, so `c++`, `node.js`,
`asp.net`, `c#`, `c/c++`, `ci/cd`, `f#` never split and stay single-token.

*Why per-alias regex over generate-variants:* generating `-`/`_`/space variants per
alias fails on MIXED separators in one occurrence (`retrieval-augmented generation`
has both `-` and space); the regex's `[-_\s]+` handles mixed uniformly. The
redundant hyphen/space alias pairs still collapse to one matcher, so they are
retired (`retrieval-augmented generation`, `sentence-transformers`).

### D2 — Boundary-guard interaction (why text is NOT collapsed)

`ASCIIBoundary` treats a leading `-` as an invalid left boundary — this is exactly
what stops `objective-c` from leaking a bare `c` (the `c developer` phrase's `c` is
preceded by `-`). Collapsing `-`→space in the text destroys that guard: `objective
c developer` then matches `c developer` and emits `c` (a regression the tests
caught). Keeping the text un-collapsed and matching separators only *inside* an
alias's own segments preserves the guard: over `objective-c developer`, the `c
developer` regex still finds `c developer`, but its match starts at a `c` preceded
by `-`, so `ASCIIBoundary` rejects it. The `.` guard (`asp.net`↛`.net`) is likewise
untouched. Regression cases (`objective-c`, `front-end`, `c developer`, all
punctuated canonicals) are locked down by tests.

### D3 — Case-preserving acronym pass, split into shared vs résumé-scoped tiers

Match a curated set of case-sensitive acronym surface forms (whole-word, over the
HTML-stripped, **case-preserved** text) → canonical slug. Canonicals must already
exist in the vocabulary (the acronym is another alias onto the same slug), so no
new facet values appear. `etl`/`grpc`/`nlp`/`llm` are NOT candidates — they are
already unambiguous bare lowercase word-aliases and resolve today.

**Uppercase is NOT always a sufficient disambiguator** — the existing guard test
`rag status trap` proves it: "RAG status" (red/amber/green project health) is
written uppercase in PM job posts, which is exactly why `rag` is excluded. Since
`skilltag` runs on ALL jobs, tagging "RAG" there would corrupt production facets.
Two tiers resolve this:

- **`sharedAcronyms`** — unambiguous even in job text; applied by the default
  `Parse` (jobs + résumés). Seed: **`ML → machine-learning`** (lower `ml` =
  millilitre, excluded; upper `ML` in an IT corpus = machine learning;
  `machine-learning` canonical exists).
- **`resumeAcronyms`** — collide with a non-tech uppercase meaning in *job* text
  but are safe in *résumés*; applied only under the résumé option. Seed:
  **`RAG → rag`** ("RAG status" is a PM-job artefact, near-absent in résumés).

`Parse` gains a variadic option: `Parse(text string, opts ...Option)`. Default
(no opts) = job-safe: separator-fix + `sharedAcronyms`. The résumé path
(`handler.ExtractResumeSkills`, the only résumé consumer) calls
`Parse(text, skilltag.WithResumeAcronyms())`, adding `resumeAcronyms`. Existing
callers (`jobderive`, …) call `Parse(text)` unchanged and never see `RAG`, so the
`rag status trap` guard and all job facets stay intact.

*Why not add lowercase `rag`/`ml` to `wordAliases`:* reintroduces the exact
ambiguity the dictionary avoids (`rag` cloth, `ml` millilitre). Case-sensitivity +
tiering is the disambiguator.

### D4 — `Parse` structure

```
func Parse(text string, opts ...Option) []string
htmlStripped := stripHTML(text)           // case preserved
acronyms := sharedAcronyms                 // + resumeAcronyms if WithResumeAcronyms()
for sf, canon := range acronyms {          // case-sensitive whole-word
    if containsCaseSensitive(htmlStripped, sf) { set[canon] }
}
norm := matchNormal(htmlStripped)          // lowercase + collapse [-_\s]+ → " "
for _, p := range phraseAliases { if Contains(norm, p.normAlias, ASCIIBoundary) { set[p.canonical] } }
for _, tok := range wordTokens(norm) { if c, ok := wordAliases[tok]; ok { set[c] } }
return sorted(set)
```

Passes union into a set (order-independent); acronym-first only so the
case-preserved text is used before it is lowercased.

## Risks / Trade-offs

- **Lost leading-`-` boundary guard (D2)** → Mitigation: targeted regression
  tests for `objective-c`, `front-end`, and any hyphenated non-skill compounds;
  the `c`/ambiguous canonicals are phrase-only, so bare-token leakage is not
  possible.
- **Acronym false positives (D3)** → Mitigation: curated, per-entry precision
  gate; case-sensitivity; document `RAG`'s trade-off; keep the list short and
  conservative.
- **Collapsing `_` could merge snake_case identifiers** (e.g. `spring_boot`) →
  this is desirable here (matches `spring boot`), but tests confirm no unintended
  merges create false hits.
- **Stale facets across the catalogue** → Mitigation: the standard
  dictionary-change ops (`cmd/backfill-derive` + `cmd/reindex`) after deploy;
  called out in tasks and the finish step. No schema change, so no migration
  risk.

## Migration Plan

1. Ship the code + tests (no schema/migration).
2. Deploy the new binary/images.
3. Run `cmd/backfill-derive` (re-derives all six dictionary facets incl. skills)
   then `cmd/reindex` on prod to refresh `jobs.skills` for existing jobs.
4. Rollback = redeploy prior image; the matcher change is additive-recall, so a
   stale index simply reverts to the old (narrower) matches — no data loss.

## Open Questions

- Final acronym list contents — start conservative (`RAG`, `ETL`, `ELT`, `gRPC`)
  and grow per-entry with the same precision gate. Not blocking.
