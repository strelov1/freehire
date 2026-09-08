## Why

`internal/job/reqextract/AGENTS.md`'s own Limitations section named this gap: the heading vocabulary is "Latin-alphabet, mostly English... Hungarian is in; more languages are an additive change to the three vocabularies." Found via this session's code audit — worth closing given the catalogue aggregates many non-English-market sources (djinni, tbank, earcu, gulftalent, bayt, seek).

## What Changes

- The requirements heading vocabulary (`requiredHeadings`, `closingHeadings`) gains four Russian entries, each verified against a real, live sample rather than guessed: "Требования" (requirements) and "Мы ждём от вас" (what we expect from you) open a required section; "Мы предлагаем" (we offer) and "Условия" (terms/conditions) close one. Measured against 500 real `tbank.ru` postings in the local dev database: 683/683/158/631 real occurrences respectively.
- "Обязанности" (duties/responsibilities, 682 real occurrences in the same sample) is deliberately NOT added — it describes what the job does, not what is required of the candidate, the same distinction that already excludes an English "what you'll do"-style heading from this vocabulary.
- Two new scenarios in `TestDerive`: a positive case (the vocabulary firing when a list follows the heading) and an honest negative case reproducing the real `tbank.ru` shape.

**A significant finding surfaced while verifying against real data, confirmed with the user before proceeding — this materially narrows what this change actually delivers:** every one of the 683 real "Требования" headings sampled is followed by a run of `<p>` paragraphs, never a `<ul>`/`<ol>` list. `Derive`'s walk only extracts items from a list; a `<p>` long enough to fail the heading-candidate test closes the section as prose before any list is found. So on this specific real, large source, this change's own vocabulary addition extracts **zero** requirements — the actual blocker for `tbank.ru` isn't language coverage, it's a structural mismatch that would affect any language. This is stated plainly in `AGENTS.md`, in this proposal, and in the test suite itself (the negative test case), rather than left to be discovered later as a silent gap between what shipped and what the commit message implied.

**Explicitly not in scope:** teaching `Derive` to recognize a `<p>`-per-item section as a list-shaped one. That is a change to the core walk shared by every language and source, materially larger and riskier than a vocabulary addition, and is written up as a separate, still-open limitation rather than folded into this change.

## Capabilities

### New Capabilities
(none)

### Modified Capabilities
- `posting-requirements-derivation`: gains an ADDED requirement describing that the heading vocabulary recognizes headings in more than one language (existing behavior for Hungarian, now also Russian), via the same transliteration the vocabulary already uses — not a new mechanism, a wider vocabulary.

## Impact

- `internal/job/reqextract/reqextract.go`: four new vocabulary entries, no change to `Derive`'s walk logic, no change to `normalizeHeading`.
- `internal/job/reqextract/reqextract_test.go`: two new `TestDerive` cases.
- `internal/job/reqextract/AGENTS.md`: Limitations section reframed (Russian added alongside Hungarian) and a new bullet documenting the `<p>`-vs-list structural gap this investigation surfaced.
- No migration, no reindex needed for this alone — `reqextract.Derive` runs at ingest/backfill time (`cmd/backfill-requirements`), and its output (`jobs.requirements_derived`) is read from Postgres directly, not indexed as a search filter (per CLAUDE.md's own note on `backfill-requirements`: "Needs no reindex").
