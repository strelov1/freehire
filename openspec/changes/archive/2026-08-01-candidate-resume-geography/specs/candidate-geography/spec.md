## ADDED Requirements

### Requirement: Candidate geography is derived through a candidate-specific entry point

The system SHALL derive a candidate's geography from the free-text location line of
their structured résumé through a dedicated entry point (`location.ParseResidence`)
that returns countries, regions, and cities, and SHALL NOT consume the job-location
parser's output (`location.Parse`) directly for candidate text. The derivation MUST be
deterministic and dictionary-only: no LLM call, no network access, and no guessing —
a location the curated dictionary cannot resolve yields nothing rather than an
approximation, the same discipline every other production facet dictionary follows.

The two parsers exist separately because the same string means different things. For a
job, a location answers "where is the work"; for a résumé it answers "where is the
person". A single parser with a flag would let one meaning be passed where the other is
expected; two types cannot be confused at a call site.

#### Scenario: A city and country on a CV resolve to a country and its region

- **WHEN** a candidate's résumé states `Valencia, Spain`
- **THEN** the derived geography is countries `[es]`, regions `[eu]`, and cities `[Valencia]`

#### Scenario: An unresolvable location yields nothing rather than a guess

- **WHEN** a candidate's résumé states a location the curated dictionary cannot place
  (e.g. `Nyarugenge District, Nyakabanda Sector`)
- **THEN** the derived countries, regions, and cities are all empty and no country is inferred

#### Scenario: Deriving candidate geography performs no I/O

- **WHEN** candidate geography is derived for any location string
- **THEN** the derivation completes without an LLM call, an HTTP request, or a database read

### Requirement: A candidate is never located globally

The candidate geography derivation SHALL NOT emit the `global` region. A job may
legitimately be open anywhere, but a person is always physically somewhere, so
`global` is never a true statement about a candidate's whereabouts. The exclusion MUST
hold regardless of which mechanism produced the region — both the bare-remote fallback
(a location that resolves no place but carries a remote marker) and the dictionary's
own `worldwide`/`anywhere`/`по всему миру` → `global` entries.

A remote marker alongside a resolvable place MUST NOT suppress that place: the place is
where the person is, and the remoteness is not geography at all.

#### Scenario: A bare remote marker yields no geography for a candidate

- **WHEN** a candidate's résumé states `Remote (GMT+3)`
- **THEN** the derived countries and regions are empty — in particular the region is not `global`

#### Scenario: An explicit worldwide word yields no geography for a candidate

- **WHEN** a candidate's résumé states `REMOTE · WORLDWIDE`
- **THEN** the derived regions are empty — the dictionary's `worldwide` → `global` mapping
  does not reach the candidate result

#### Scenario: A real macro-region without a country is kept

- **WHEN** a candidate's résumé states `EU / Remote`
- **THEN** the derived regions are `[eu]` and the countries are empty — a candidate stating
  they are in the EU is a true claim about where they are

#### Scenario: A place stated alongside a remote marker survives

- **WHEN** a candidate's résumé states a resolvable place together with a remote marker
- **THEN** the derived geography names that place and is not emptied or globalized

### Requirement: Work mode is not part of a candidate's location

The candidate geography derivation SHALL NOT emit a work-mode value. How a person is
willing to work is a preference, not a fact about where they are, and it already has a
home in the profile's `location_preferences.work_modes`. A work-mode marker in the CV
location line MUST be used only to the extent that it helps resolve the place, and MUST
NOT be persisted as part of the derived geography.

#### Scenario: A hybrid marker does not become candidate geography

- **WHEN** a candidate's résumé states a location carrying an explicit work-mode marker
- **THEN** the derived geography carries countries, regions, and cities only, and no
  work-mode value is stored on the user

### Requirement: Derived geography is written with the structure it came from

The derived geography SHALL be computed and persisted in the same write that persists
the structured résumé, stamped with the same résumé upload time. It MUST therefore
inherit that write's monotonic guard: a background derivation for a résumé that has
since been replaced MUST leave both the structure and the geography untouched, so the
two can never describe different CVs.

Because the derivation is deterministic and costs no I/O, it MUST be computed
synchronously on that write rather than scheduled as separate background work — a
separate schedule would introduce a state in which a user has a structure but not yet
the geography derived from it.

#### Scenario: Persisting a structure persists its geography

- **WHEN** a structured résumé stating a resolvable location is persisted for a user
- **THEN** the user's derived countries, regions, and cities are stored in the same write

#### Scenario: A superseded derivation writes neither structure nor geography

- **WHEN** a background extraction completes for a résumé that has since been replaced by
  a newer upload
- **THEN** neither the structured résumé nor the derived geography is written, and the
  newer CV's values are left intact

#### Scenario: Deleting the résumé clears the derived geography

- **WHEN** a signed-in user deletes their stored résumé
- **THEN** the derived countries, regions, and cities are cleared along with the structure

### Requirement: Unknown geography is distinguishable from unresolved geography

The stored derived geography SHALL distinguish "we do not know where this candidate is"
from "the candidate stated a place the dictionary could not resolve". Absent geography
(no CV, no current structured résumé, or a structure stating no location) MUST be
represented as absent; a stated-but-unresolved location MUST be represented as an empty
resolved set. The two MUST NOT collapse into one value.

This keeps the dictionary's coverage gap measurable on live data for as long as the
column exists, rather than hiding it behind a value that also means "not derived yet".

#### Scenario: A user with no current structured résumé has absent geography

- **WHEN** a user has no structured résumé, or one that no longer matches their current CV
- **THEN** their derived geography is absent — not an empty set of countries

#### Scenario: A stated but unresolvable location is an empty resolved set

- **WHEN** a user's structured résumé states a location the dictionary cannot resolve
- **THEN** their derived countries are an empty set, distinguishable from absent

### Requirement: Derived geography is reconcilable by a re-runnable worker

The system SHALL provide a run-once-and-exit worker that re-derives the stored geography
from already-persisted structured résumés. The worker MUST require only a database
connection — no LLM, no object storage, and no other network dependency — so that it is
cheap and safe to re-run after any change to the location dictionary, in the same way
the job facets are re-derived.

The worker MUST be idempotent: a second run over unchanged data and an unchanged
dictionary MUST leave every stored value identical. It MUST process only users whose
structured résumé currently describes their stored CV, since deriving geography from a
superseded structure would circumvent the staleness rule.

#### Scenario: Re-running the worker changes nothing

- **WHEN** the worker is run twice in succession with no intervening change to the stored
  structures or the dictionary
- **THEN** the second run leaves every user's derived geography byte-identical to the first

#### Scenario: A dictionary change is picked up by a re-run

- **WHEN** the location dictionary gains the ability to resolve a country it previously
  could not, and the worker is re-run
- **THEN** users whose stated location names that country gain the resolved country

#### Scenario: A user with a superseded structure is skipped

- **WHEN** the worker runs over a user whose structured résumé no longer matches their
  current CV
- **THEN** that user is skipped and their derived geography is not written from the stale structure

#### Scenario: The worker runs without LLM or object-storage configuration

- **WHEN** the worker is started with only a database connection configured
- **THEN** it runs to completion rather than failing on a missing LLM or storage setting

### Requirement: An asserted location outranks a derived one

Where a consumer needs the candidate's country and the candidate has asserted one in
their profile, the asserted value SHALL be used. A derived country MAY fill the gap only
when the candidate has asserted nothing. A derived geography naming more than one country
MUST be treated as no answer rather than resolved by picking one, preserving the
never-guess discipline: the point of the derivation is to supply a fact, and an ambiguous
derivation is not one.

#### Scenario: An asserted base country wins over a derived one

- **WHEN** a candidate has stated a base country in their profile and their CV derives a
  different country
- **THEN** consumers use the stated country

#### Scenario: A derived country fills an unstated base

- **WHEN** a candidate has stated no base country and their CV derives exactly one country
- **THEN** consumers use the derived country

#### Scenario: An ambiguous derivation is not resolved by guessing

- **WHEN** a candidate has stated no base country and their CV derives more than one country
- **THEN** consumers receive no country rather than one of the derived candidates
