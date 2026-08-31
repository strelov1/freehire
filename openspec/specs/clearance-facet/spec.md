# clearance-facet Specification

## Purpose

The `requires_clearance` signal end to end: what counts as a stated government
security-clearance requirement (UK SC/DV/BPSS, US Secret/TS-SCI/polygraph, AU
baseline/NV1), how it is detected dict-only from a posting's description, how it is
stored as a tri-state that is only ever `true` or `NULL`, and how it is served,
filtered and backfilled.

It exists so a candidate who cannot hold a clearance can take those postings out of
their results, and a candidate who holds one can see nothing else.

## Requirements

### Requirement: A stated government clearance requirement is detected dict-only

The system SHALL detect, from a posting's description alone, whether it states a
government security-clearance requirement, and SHALL do so from a curated list of
anchored phrases — never from an LLM, and never by guessing. A description that
states no clearance requirement SHALL yield *unknown*, not a negative: the
dictionary emits nothing for what it cannot resolve, the same discipline every
other facet dictionary follows.

The list SHALL cover the schemes the catalogue actually carries: the UK
(`SC clearance`, `SC cleared`, `DV clearance`, `DV cleared`, `CTC clearance`,
`BPSS`, `security vetting`, `developed vetting`), the US (`security clearance`,
`secret clearance`, `top secret clearance`, `TS/SCI`, `SCI clearance`,
`polygraph`, `public trust clearance`, `DoD clearance`), Australia
(`baseline clearance`, `NV1`, `NV2`, `negative vetting`, `AGSVA`), and the
scheme-neutral forms (`active clearance`, `current clearance`, `security-cleared`,
`clearance required`, `must hold a clearance`).

Bare short tokens SHALL NOT appear on the list. `SC`, `DV`, and `L` collide with
ordinary words and unrelated initialisms, so each is listed only in a longer
anchoring form. This is the same precision-over-recall trade `eligibility.go`
already documents for the geography phrases.

#### Scenario: A UK clearance requirement is detected

- **WHEN** a description says "You must hold or be eligible for SC clearance"
- **THEN** the posting is marked as requiring a clearance

#### Scenario: A US clearance requirement is detected

- **WHEN** a description says "Active TS/SCI with CI Polygraph required"
- **THEN** the posting is marked as requiring a clearance

#### Scenario: An Australian clearance requirement is detected

- **WHEN** a description says "Must hold a current NV1 clearance"
- **THEN** the posting is marked as requiring a clearance

#### Scenario: A bare short token does not fire

- **WHEN** a description says "We are an SC-registered charity" or "the DV team
  owns deployment"
- **THEN** the posting is NOT marked as requiring a clearance

#### Scenario: Silence yields unknown, not a negative

- **WHEN** a description mentions no clearance scheme at all
- **THEN** the clearance signal is unknown — neither `true` nor `false`

### Requirement: A labelled clearance field is detected as well as clearance prose

The system SHALL additionally detect the labelled-field form ATS postings use to
state the requirement as structured text rather than prose — a `clearance` label
followed by a separator and a value, as in `Clearance: Secret`,
`Clearance Level: Public Trust`, `Clearance Required: Yes`,
`CLEARANCE TYPE: Polygraph`.

A labelled field SHALL mark the posting only when its value names a clearance or
asserts one is required. A label whose value denies the requirement
(`Clearance Required: No`, `Clearance: None`, `Clearance: N/A`) SHALL NOT mark it.

This rule exists because a phrase list alone misses this form: in the sampled
`clearance` rows it accounted for roughly a fifth of all true positives.

#### Scenario: A labelled field naming a clearance is detected

- **WHEN** a description contains "Clearance: TS with SCI eligibility"
- **THEN** the posting is marked as requiring a clearance

#### Scenario: A labelled field asserting the requirement is detected

- **WHEN** a description contains "CLEARANCE REQUIRED FOR START: Yes"
- **THEN** the posting is marked as requiring a clearance

#### Scenario: A labelled field denying the requirement does not fire

- **WHEN** a description contains "Clearance Required: No" or "Clearance: None"
- **THEN** the posting is NOT marked as requiring a clearance

### Requirement: The description is read as visible text, not as markup

Descriptions are stored as HTML, so the system SHALL strip the markup before
matching. Tags routinely land between a label and its value, and matching the raw
markup reads how a posting is typeset rather than what it says.

A phrase that is itself a field LABEL — immediately followed by a colon — SHALL
assert nothing on its own; the label's value decides. Otherwise
`Security Clearance: None/Not Required` marks itself, and the denial cannot rescue
it, because stripping the markup puts the value on the next line and the
sentence-scoped negation check stops at the line break.

#### Scenario: Markup between a label and its denial

- **WHEN** a description contains
  `<p><b>Security Clearance: </b></p>None/Not Required`
- **THEN** the posting is NOT marked as requiring a clearance

#### Scenario: Markup between a label and its value

- **WHEN** a description contains
  `<p><b>Clearance Level Must Currently Possess:</b></p><p>Top Secret/SCI</p>`
- **THEN** the posting IS marked as requiring a clearance

### Requirement: A denied clearance requirement cancels the signal

The system SHALL NOT mark a posting as requiring a clearance when the description
denies the requirement — `no security clearance required`,
`security clearance is not required`, `this role does not require a clearance`,
`no clearance needed`.

A denial cancels the sentence it sits in, not the whole description: an anchor
asserted in a different sentence still marks the posting. Sentence scope is what
`eligibility.go` already implements and it is enough here — a regex for the denial
forms matched 0 of 923 sampled `clearance` rows, and 0 of the 269 that carried an
anchor, so a description that both denies and asserts is not a shape the catalogue
produces. Widening the scope to the whole description would trade a measured zero
against the risk of one stray "not" suppressing a genuine requirement.

#### Scenario: An explicit denial cancels the signal

- **WHEN** a description says "No security clearance is required for this role"
- **THEN** the posting is NOT marked as requiring a clearance

#### Scenario: A denial does not suppress an anchor in another sentence

- **WHEN** a description says "No security clearance is required." and separately
  says "You must hold an active TS/SCI clearance."
- **THEN** the posting IS marked as requiring a clearance

### Requirement: Unrelated senses of "clearance" do not fire

The system SHALL NOT mark a posting on the strength of the word `clearance` in a
sense unrelated to government vetting — `customs clearance`, `medical clearance`,
`clearance sale`, `security clearance specialist` in the medical-billing sense.
Only the anchored phrases and the labelled field fire; a bare `clearance` token
never does.

#### Scenario: Customs clearance does not fire

- **WHEN** a description says "handle inbound/outbound customs clearance"
- **THEN** the posting is NOT marked as requiring a clearance

#### Scenario: Medical clearance does not fire

- **WHEN** a description says "medical clearance is required before your start
  date"
- **THEN** the posting is NOT marked as requiring a clearance

### Requirement: An obtainable clearance counts as required

The system SHALL mark a posting that asks the candidate to be *able to obtain* a
clearance (`must be able to obtain a security clearance`,
`eligible for SC clearance`, `Clearance Required: Ability to Obtain Public Trust`)
the same as one demanding an existing clearance.

Eligibility for a clearance turns on nationality and residency history, so a
candidate who cannot hold one cannot obtain one either. Serving these as
unrestricted would leave them in exactly the lane the filter exists to clear.

#### Scenario: An obtainable clearance is marked

- **WHEN** a description says "Must be able to obtain and maintain a US Secret
  clearance"
- **THEN** the posting is marked as requiring a clearance

### Requirement: The clearance signal is a stored, tri-state column derived on every write path

The system SHALL store the signal in `jobs.requires_clearance`, a nullable
boolean: `true` when the description states a requirement, `NULL` when it states
nothing. It SHALL NOT write an explicit `false` — a denial produces the absence of
a `true`, not an assertion, because the dictionary cannot distinguish "this
posting says no clearance is needed" from "this posting is silent" reliably enough
to serve the difference.

The column SHALL be computed by `jobderive` and derived through the `Job` aggregate
factory (`job.New`) on every write path — ingest, moderator authoring, and
Telegram — so the three cannot diverge.

#### Scenario: A clearance posting stores true

- **WHEN** a posting whose description states a clearance requirement is written
- **THEN** `jobs.requires_clearance` is `true`

#### Scenario: A silent posting stores NULL

- **WHEN** a posting whose description states no clearance requirement is written
- **THEN** `jobs.requires_clearance` is `NULL`

#### Scenario: The moderator path derives identically to ingest

- **WHEN** a moderator-authored posting and a board-ingested posting carry the
  same description
- **THEN** they resolve the same `requires_clearance` value, because both
  construct their `Job` through the aggregate factory

### Requirement: The facet is served and filterable

The public read model SHALL serve the signal as `requires_clearance`, omitted when
unknown, sourced from the `jobs` column only.

`GET /api/v1/jobs` SHALL accept an optional `requires_clearance` query parameter.
`requires_clearance=false` SHALL return the postings that are NOT marked — both
the explicitly-unmarked and the unknown — because a searcher asking to exclude
clearance jobs wants everything the system does not know to require one.
`requires_clearance=true` SHALL return only the marked postings, which serves the
opposite audience: a cleared candidate searching for the work they are uniquely
eligible for. Omitting the parameter SHALL behave exactly as today.

The Meilisearch index SHALL declare a matching filterable attribute.

#### Scenario: Excluding clearance jobs returns the unknowns too

- **WHEN** a caller requests `GET /api/v1/jobs?requires_clearance=false`
- **THEN** the results contain postings whose `requires_clearance` is `NULL` as
  well as those explicitly not marked, and no posting marked `true`

#### Scenario: Requesting only clearance jobs returns the marked ones

- **WHEN** a caller requests `GET /api/v1/jobs?requires_clearance=true`
- **THEN** every result is marked as requiring a clearance

#### Scenario: An unknown facet is omitted from the wire shape

- **WHEN** a posting with `requires_clearance = NULL` is served
- **THEN** the response object carries no `requires_clearance` key

#### Scenario: Omitting the filter changes nothing

- **WHEN** a caller requests `GET /api/v1/jobs` with no `requires_clearance`
  parameter
- **THEN** the results include clearance and non-clearance postings alike

### Requirement: The filterable attribute reaches the live index before the binary requesting it

The Meilisearch settings patch declaring `requires_clearance` filterable SHALL be
applied to the live index BEFORE the binary that requests the facet is deployed.

A binary that requests a filterable attribute the live index has not declared
hard-500s `/api/v1/jobs/facets` for every caller, signed in or not. This is a
documented hazard of this codebase, not a hypothetical.

#### Scenario: Settings precede the binary

- **WHEN** the change is rolled out
- **THEN** the index settings declare the attribute before the new binary serves
  traffic, and `/api/v1/jobs/facets` never 500s during the rollout

### Requirement: The existing catalogue is backfilled from a search-named candidate set

The backfill SHALL re-derive only the postings a Meilisearch query names as
candidates — the `clearance` token, plus the anchors that do not contain the word
(`ts/sci`, `polygraph`, `bpss`, `vetting`, `agsva`) — rather than walking the whole
catalogue.

It SHALL be idempotent and resumable: a row whose derived value already matches is
not written, so a re-run costs nothing and stopping it mid-way is free.

A full catalogue pass SHALL NOT be used. It runs ~15 hours, and a `description`
predicate over the whole table de-TOASTs 8M rows — a known production trap.

#### Scenario: Only candidates are touched

- **WHEN** the backfill runs
- **THEN** it reads and updates only the search-named candidate rows, leaving the
  rest of the catalogue unread

#### Scenario: A re-run writes nothing

- **WHEN** the backfill is run a second time with no intervening changes
- **THEN** it writes no rows

### Requirement: The backfill is followed by a full index rebuild

Backfilling the column SHALL be followed by a full Meilisearch rebuild, because the
incremental push only sends documents whose `content_hash` moved and the new column
is not part of that hash. Without the rebuild the facet is filterable but empty for
every pre-existing posting, which reads as "no job requires a clearance" rather than
as "the index has not caught up".

This is the trap `is_tech` already fell into and it is documented in the codebase;
the rebuild is a required step of this change, not an optimisation.

#### Scenario: Backfilled rows reach the index

- **WHEN** the backfill has marked the catalogue and a full rebuild has run
- **THEN** filtering `requires_clearance=true` returns the marked pre-existing
  postings, not only the ones ingested since the deploy
