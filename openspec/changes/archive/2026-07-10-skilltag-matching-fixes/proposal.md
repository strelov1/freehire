## Why

The deterministic skill-tag matcher (`internal/skilltag`) misses two everyday
classes of real skills, which silently under-counts a candidate's or a job's
stack. This matters now because the résumé-centric analysis being built next
derives a user's skills **solely** from their résumé text — so every miss is a
lost skill in coverage and the ATS score, with no manual entry to compensate.

Two concrete gaps, both from the matcher mechanics (not missing vocabulary):
- **Hyphen/underscore joins don't match.** `distributed-systems` misses the
  `distributed systems` phrase alias because `normalize()` only strips HTML and
  lowercases — the hyphen survives, and the word pass splits on it.
- **Uppercase acronyms don't match.** `RAG` never resolves to
  retrieval-augmented-generation because the matcher lowercases everything (so
  the acronym signal is lost) and the bare `rag` alias is deliberately excluded
  as ambiguous (`rag` = cloth / RAG-status).

The fix stays **deterministic and precision-first** — no embeddings, no fuzzy
matching. `skilltag` feeds the production `jobs.skills` facet, so guessing would
corrupt facet precision; the right lever is better exact-matching rules plus a
curated acronym list, not semantic similarity.

## What Changes

- **Separator-insensitive multi-word matching.** Treat `-` and `_` between
  alphanumerics as equivalent to a space, applied consistently to both the input
  text and the phrase aliases, so `distributed-systems`, `distributed_systems`,
  and `distributed systems` all resolve to the same canonical. Punctuation that
  is part of a canonical token (`.`, `#`, `+`, `/` — as in `c++`, `node.js`,
  `asp.net`, `c#`) is **untouched**.
- **Case-preserving acronym pass.** A new pass over the original-case
  (HTML-stripped) text matches a curated, precision-vetted acronym dictionary by
  exact uppercase surface form (e.g. `RAG`, `ETL`, `ELT`, `gRPC`), so the
  acronym resolves while its ambiguous lowercase form does not. Each acronym is
  admitted only when its uppercase form is confidently unambiguous in the
  IT-jobs/résumé corpus.
- **`Parse` restructured** to run the acronym pass over case-preserved text
  before the existing lowercase phrase and word passes; results still union,
  dedup, and sort (invariant preserved).
- **No vocabulary redefinition** beyond the new curated acronym list; existing
  `wordAliases`/`phraseAliases` behavior is preserved (punctuated canonicals
  still match; no new false positives).

## Capabilities

### New Capabilities
- `skill-tag-matching`: the resolution rules of the deterministic skill-tag
  matcher — separator-insensitive multi-word matching, case-preserving acronym
  matching, and the precision-first "curated only, never guess" invariant.

### Modified Capabilities
<!-- none: the deterministic-facets invariant (dicts are the sole source of the
     six facets) is unchanged; only the matcher's resolution rules are new. -->

## Impact

- **Code:** `internal/skilltag/skilltag.go` (`normalize` + `Parse` restructure;
  new acronym pass), `internal/skilltag/dictionaries.go` (curated acronym list;
  possibly retire now-redundant duplicate hyphen/space alias pairs),
  `internal/skilltag/skilltag_test.go` + `dictionaries_test.go` (new cases +
  invariant). Possibly a small helper in `internal/wordmatch` for
  case-sensitive whole-word matching if the acronym pass needs it.
- **Consumers:** `Parse` becomes variadic (`Parse(text, opts...)`), so `jobderive`
  and the facet pipeline stay on `Parse(text)` unchanged; only the résumé path
  `handler.ExtractResumeSkills` opts in via `skilltag.WithResumeAcronyms()`.
- **Ops (dictionary-change convention):** existing jobs carry stale
  `jobs.skills`; after deploy run `cmd/backfill-derive` then `cmd/reindex` to
  re-derive the skills facet across the catalogue (same as the geography/skills
  dictionary-change procedure in AGENT.md). No schema/migration change.
