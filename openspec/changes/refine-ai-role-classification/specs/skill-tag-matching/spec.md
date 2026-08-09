## MODIFIED Requirements

### Requirement: Case-preserving acronym matching

The matcher SHALL resolve a curated set of technology acronyms by their exact
case-sensitive surface form, matched as whole words over the original-case text,
while their ambiguous lowercase forms SHALL NOT resolve. Each acronym SHALL map to
a canonical that already exists in the vocabulary (an acronym is an additional
alias, never a new facet value).

Acronyms SHALL be split into three tiers: a **shared** set applied to all text
(jobs and résumés), a **résumé-scoped** set applied only when the caller opts in
for résumé parsing, and a **category-scoped** set applied to job text only when
the caller supplies a job category present in that acronym's own allow-list. An
acronym whose uppercase form is ambiguous in job descriptions generally (e.g.
"RAG status") but unambiguous within a specific job category SHALL be
category-scoped rather than omitted from job parsing entirely; an acronym with no
such safe category SHALL remain résumé-scoped.

`RAG` SHALL be category-scoped, resolving to `rag` (retrieval-augmented
generation) on job postings whose category is `ai_engineering` or `ml_ai`, and
SHALL remain résumé-scoped for résumé parsing regardless of any category.

#### Scenario: Shared acronym resolves everywhere
- **WHEN** any text contains "ML" as a standalone token
- **THEN** the matcher emits "machine-learning"

#### Scenario: Résumé-scoped acronym resolves in résumé mode
- **WHEN** a résumé is parsed with the résumé option and contains "RAG"
- **THEN** the matcher emits "rag" (retrieval-augmented generation)

#### Scenario: Category-scoped acronym resolves for its allow-listed job category
- **WHEN** job text in category `ai_engineering` contains "RAG" as a standalone token
- **THEN** the matcher emits "rag"

#### Scenario: Category-scoped acronym does not resolve outside its allow-list
- **WHEN** job text in category `backend` contains "RAG" — including "RAG status"
- **THEN** the matcher does NOT emit "rag"

#### Scenario: Category-scoped acronym does not resolve when no category is supplied
- **WHEN** default (job) parsing with no category option sees "RAG"
- **THEN** the matcher does NOT emit "rag"

#### Scenario: Ambiguous lowercase form does not resolve
- **WHEN** the text contains the lowercase word "rag" or "ml"
- **THEN** the matcher does NOT emit the corresponding canonical

#### Scenario: Acronym matched as a whole word only
- **WHEN** "ML" appears embedded in a larger token (e.g. "HTML")
- **THEN** it does NOT emit "machine-learning"
