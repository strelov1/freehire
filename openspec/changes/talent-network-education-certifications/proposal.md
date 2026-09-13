## Why

`internal/candidate/talentnetwork.CandidateCard` (the public, anonymous Talent Network
card) withholds education and certifications entirely today. Not because the data is
missing — `resumeextract.Structured.Education`/`.Certifications` already flow from CV
upload through to storage — but because the card's own rule ("every string is a
dictionary-resolved value or a date, never a string the candidate typed") has nothing that
resolves them: the education-level dictionary that exists (`internal/job/jobfacts`) lives
in a block `candidate` may not import, and certifications have no dictionary at all. This
is exactly the follow-up `card.go`'s header comment and the `talent-network-catalog` spec
already call out as future work. Recruiters reading a card lose a real signal (level of
education, professional certifications) that a candidate would want shown, and the gap has
a known, bounded fix.

## What Changes

- New package `internal/dict/edulevel` (layer 2) resolves free-text degree wording to the
  closed vocabulary `internal/dict/vocab.EducationLevelValues` (`bachelor`/`master`/`phd`;
  `none` does not apply to a candidate's own history and is never emitted here). It exposes
  two entry points, mirroring the existing `internal/dict/location` `Parse`/`ParseResidence`
  split for two different callers reading the same underlying vocabulary:
  - the JOB-POSTING-requirement matcher, extracted unchanged from
    `internal/job/jobfacts.EducationLevel`'s regex/alias logic;
  - a new CV-DEGREE-shorthand matcher, tuned for résumé wording ("BSc", "MSc Computer
    Science", "PhD in Physics", "MBA") that the requirement-matcher's possessive-framed
    regexes ("bachelor's degree") do not recognize.
  An input that matches neither degree level resolves to empty, exactly like an unresolved
  job title in `classify.Parse` today.
- `internal/job/jobfacts.EducationLevel` becomes a thin wrapper: its existing
  `hardRequirementText` preprocessing (which depends on `internal/job/reqextract` and
  therefore must stay in the `job` block) stays as-is, then delegates the actual level
  match to the new `dict/edulevel` job-posting entry point. Its one caller
  (`internal/job/jobderive/jobderive.go:234`) is unaffected.
- New package `internal/dict/certification` (layer 2): a curated alias→canonical
  dictionary of known professional certifications (AWS, PMP, CKA, CISSP, etc.), following
  `internal/dict/skilltag`'s `Canonicalize` shape already used by `card.go`. An
  unrecognized certification name resolves to nothing — dict-only, never guessed.
- `CandidateCard` gains two fields: `Education []EducationEntry` (`{Level string, Year
  *perioddate.PeriodDate}`, one entry per CV education item whose degree resolves — an
  item that does not resolve is dropped, not kept under an empty label, since (unlike a
  role) a partially-unlabelled education line has no "gap reads worse than absence"
  argument in its favor) and `Certifications []string` (canonical, deduplicated). Neither
  field ever carries an institution name, an issuer, a certification date, or a field of
  study.
- Talent Network profile page (`web/src/routes/talent/[handle]/+page.svelte`) gains two
  new sections, "Education" and "Certifications", rendered the same way as the existing
  "Skills" section (chip list, hidden when empty). New label maps
  `EDUCATION_LEVEL_LABELS`/`CERTIFICATION_LABELS` in `web/src/lib/labels.ts`.

## Capabilities

### New Capabilities

None — this extends what the public card discloses; it does not introduce a new
capability of its own.

### Modified Capabilities

- `talent-network-catalog`: "A public response carries no free text from a CV" currently
  states that languages, certifications and education are withheld because no reachable
  dictionary resolves them. That requirement's text and its "CV field that names the
  employer" scenario (which lists "the institution, the degree line... a certification" as
  places a name must never leak from) both need updating: certifications and education
  level become part of what the card CARRIES, under the same "dictionary-resolved or
  dropped" rule already governing skills — while institution name, certification issuer,
  certification date and field of study remain explicitly withheld, same as an employer
  name.

## Impact

- New packages: `internal/dict/edulevel`, `internal/dict/certification`.
- Modified: `internal/job/jobfacts` (its `EducationLevel` function's internals only — same
  signature, same caller), `internal/candidate/talentnetwork/card.go` (`CandidateCard`,
  `ProjectCard`), `web/src/routes/talent/[handle]/+page.svelte`, `web/src/lib/labels.ts`.
- No database migration, no CV-extraction schema change, no API route change beyond the
  card's own JSON payload gaining two optional fields.
