# skill-tag-matching Specification

## Purpose
TBD - created by archiving change skilltag-matching-fixes. Update Purpose after archive.
## Requirements
### Requirement: Separator-insensitive multi-word matching

The skill-tag matcher SHALL treat `-` and `_` between alphanumerics as equivalent
to a space when resolving multi-word terms, so that hyphenated, underscored, and
space-separated forms of a term all resolve to the same canonical skill. This
normalization SHALL apply consistently to both the input text and the phrase
aliases.

#### Scenario: Hyphenated multi-word term resolves
- **WHEN** the text contains "distributed-systems"
- **THEN** the matcher emits the same canonical ("distributed-systems") it emits for "distributed systems"

#### Scenario: Underscored multi-word term resolves
- **WHEN** the text contains "distributed_systems"
- **THEN** the matcher emits the same canonical it emits for "distributed systems"

#### Scenario: Space form still resolves
- **WHEN** the text contains "distributed systems"
- **THEN** the matcher emits its canonical (no regression from the normalization)

### Requirement: Punctuated canonicals are preserved

The separator normalization SHALL collapse only `-`/`_` (and whitespace); the
punctuation characters `.`, `#`, `+`, and `/` that are part of a canonical token
SHALL be left intact, so punctuated skills continue to resolve and do not gain
false positives.

#### Scenario: Plus/hash/dot canonicals still match
- **WHEN** the text contains "c++", "c#", "node.js", or "asp.net"
- **THEN** each resolves to its canonical (cpp, csharp, nodejs, dotnet) exactly as before

#### Scenario: Suffix of a punctuated token is not a false match
- **WHEN** the text contains "asp.net"
- **THEN** it does NOT additionally emit a ".net"-only match that the leading-dot boundary guard forbids

#### Scenario: Hyphen compound does not leak an ambiguous single-letter canonical
- **WHEN** the text contains "objective-c" with no other C signal
- **THEN** the matcher does NOT emit the "c" canonical

### Requirement: Case-preserving acronym matching

The matcher SHALL resolve a curated set of technology acronyms by their exact
case-sensitive surface form, matched as whole words over the original-case text,
while their ambiguous lowercase forms SHALL NOT resolve. Each acronym SHALL map to
a canonical that already exists in the vocabulary (an acronym is an additional
alias, never a new facet value).

Acronyms SHALL be split into two tiers: a **shared** set applied to all text
(jobs and résumés), and a **résumé-scoped** set applied only when the caller opts
in for résumé parsing. An acronym whose uppercase form is ambiguous in job
descriptions (e.g. "RAG status") SHALL be résumé-scoped so it never tags jobs.

#### Scenario: Shared acronym resolves everywhere
- **WHEN** any text contains "ML" as a standalone token
- **THEN** the matcher emits "machine-learning"

#### Scenario: Résumé-scoped acronym resolves only in résumé mode
- **WHEN** a résumé is parsed with the résumé option and contains "RAG"
- **THEN** the matcher emits "rag" (retrieval-augmented generation)

#### Scenario: Résumé-scoped acronym never tags job text
- **WHEN** default (job) parsing sees "RAG" — including "RAG status"
- **THEN** the matcher does NOT emit "rag"

#### Scenario: Ambiguous lowercase form does not resolve
- **WHEN** the text contains the lowercase word "rag" or "ml"
- **THEN** the matcher does NOT emit the corresponding canonical

#### Scenario: Acronym matched as a whole word only
- **WHEN** "ML" appears embedded in a larger token (e.g. "HTML")
- **THEN** it does NOT emit "machine-learning"

### Requirement: Precision-first, curated-only resolution

The matcher SHALL remain a curated dictionary that resolves only known aliases
(including the new separator and acronym rules) and SHALL emit nothing for terms
it cannot resolve — it never guesses, and no fuzzy or semantic similarity is
used. Results SHALL stay deduplicated and sorted.

#### Scenario: Unknown term emits nothing
- **WHEN** the text contains a term with no curated alias
- **THEN** the matcher emits no canonical for it

#### Scenario: Deterministic, deduplicated, sorted output
- **WHEN** the same skill appears multiple times and in mixed separator/case forms
- **THEN** its canonical appears exactly once and the result is sorted (identical across runs)

