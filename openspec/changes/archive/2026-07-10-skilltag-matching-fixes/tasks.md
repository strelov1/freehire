## 1. Separator-insensitive multi-word matching

- [x] 1.1 RED: add `skilltag_test.go` cases — "distributed-systems" and "distributed_systems" resolve to the same canonical as "distributed systems"; mixed forms in one text dedup to one; the space form still resolves.
- [x] 1.2 RED: add regression cases — punctuated canonicals "c++", "c#", "node.js", "asp.net", "c/c++", "f#" still resolve; "asp.net" does NOT leak ".net"; "objective-c" (no other C signal) does NOT emit "c"; a hyphen compound like "front-end" emits no spurious canonical.
- [x] 1.3 GREEN: introduce a shared match-normal form in `skilltag.go` — after HTML strip + lowercase, collapse `[-_\s]+` to a single space (leave `.`/`#`/`+`/`/` intact). Apply to the input text; precompute the normalized form of every `phraseAliases` entry once at package init and match against it. Keep `wordAliases` word-pass as is (tokens already split on `-`/`_`).
- [x] 1.4 REFACTOR: retire any now-redundant duplicate alias pairs whose only difference was hyphen-vs-space (e.g. paired "retrieval augmented generation" / "retrieval-augmented generation") — the normal form makes one entry cover both. Keep tests green.

## 2. Case-preserving acronym pass (shared + résumé-scoped tiers)

- [x] 2.1 RED: shared — "ML" resolves to "machine-learning" in default parsing; "ML" inside "HTML" does NOT; lowercase "ml" does NOT. Résumé-scoped — `Parse(text, WithResumeAcronyms())` on "RAG" resolves "rag"; DEFAULT `Parse` on "RAG"/"RAG status" does NOT emit "rag" (existing `rag status trap` guard stays green).
- [x] 2.2 GREEN: add `sharedAcronyms` (ML→machine-learning) + `resumeAcronyms` (RAG→rag) to `dictionaries.go` — each value an EXISTING canonical (no new facet value). Add a case-sensitive whole-word matcher (small `internal/wordmatch` helper or case-sensitive boundary). Add `type Option` + `WithResumeAcronyms()`; make `Parse` variadic `Parse(text string, opts ...Option)` (existing `Parse(text)` callers unchanged).
- [x] 2.3 GREEN: restructure `Parse` — strip HTML (case-preserved) → acronym pass (shared + résumé tier when opted) → match-normal → phrase → word → union/dedup/sort. Confirm sort+dedup, `rag status trap`, and AI-vocab guards stay green.
- [x] 2.4 GREEN: wire the résumé consumer — `handler.ExtractResumeSkills` calls `skilltag.Parse(up.Text, skilltag.WithResumeAcronyms())`; job callers (`jobderive`, …) stay on `Parse(text)`.

## 3. Dictionary invariant + docs

- [x] 3.1 Extend `dictionaries_test.go` (or the invariant test) so every `sharedAcronyms`/`resumeAcronyms` value is a valid canonical slug reachable via an existing alias — no acronym introduces a brand-new facet value.
- [x] 3.2 Update the package doc comment in `skilltag.go` to describe the two new resolution rules (separator-insensitivity, case-preserving acronyms) and the precision-first invariant.

## 4. Verify

- [x] 4.1 `go build ./... && go vet ./... && go test ./internal/skilltag/ ./...`; confirm no regressions across the suite. Record the required post-deploy ops in the finish step: run `cmd/backfill-derive` then `cmd/reindex` on prod to re-derive `jobs.skills` for existing jobs (dictionary-change convention).
