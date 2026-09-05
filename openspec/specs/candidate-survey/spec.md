# candidate-survey Specification

## Purpose
The candidate's self-reported segmentation answers — how far along their job search is,
the single biggest thing in its way, and what they earn today. These describe the
candidate to US and to nobody else: they reach no employer, no job search and no
ranking. What a candidate WANTS to be paid is a screening answer, not one of these.

## Requirements

### Requirement: The questionnaire records self-reported segmentation facts

The system SHALL store, per account, three self-reported facts that no CV and no filter can
supply: the candidate's job-search stage, their single biggest challenge, and their current
income. Each fact is independently optional and its absence MUST be representable as
"unstated" — never as a default, a zero, or a guess.

The questionnaire is deliberately separate from the search profile (`user_profiles`) and
from the ATS screening answers (`screening_answers`). It MUST NOT influence job search,
ranking, or matching. Storing it beside the search profile would misrepresent it as a
filter; storing it beside the screening answers would misrepresent it as something an
employer sees.

Current income is recorded as an amount, a currency, and a period — the same triple, and
the same vocabularies, that `screening_answers` uses for desired salary — so the two figures
remain comparable without conversion.

#### Scenario: A stated fact is stored and read back

- **WHEN** an authenticated user submits a job-search stage and a biggest challenge
- **THEN** both are stored against their account and returned unchanged on the next read

#### Scenario: An unanswered question stays unstated

- **WHEN** a user submits only a job-search stage, leaving challenge and income unanswered
- **THEN** the stored challenge and income remain null, distinguishable from any answered value

#### Scenario: The questionnaire does not reach job search

- **WHEN** a user has answered every questionnaire question
- **THEN** their job search results, ordering, and facet counts are identical to those of a user who answered none

### Requirement: Questionnaire answers are drawn from closed vocabularies

The system SHALL validate `job_search_stage` and `biggest_challenge` against closed
vocabularies held in the shared vocabulary package, and SHALL reject a value outside them.
Validation happens in application code, not as a database constraint, matching how the
adjacent screening answers validate their salary period.

The challenge vocabulary includes an explicit "other" member, and only when that member is
selected MAY a free-text note accompany it. A note submitted alongside any other challenge
MUST be rejected, so the note can never contradict the coded answer.

#### Scenario: An unknown stage is rejected

- **WHEN** a request carries a job-search stage that is not in the vocabulary
- **THEN** the request is rejected and nothing is stored

#### Scenario: A free-text note accompanies "other"

- **WHEN** a user selects the "other" challenge and supplies a note
- **THEN** both the coded challenge and the note are stored

#### Scenario: A note without "other" is rejected

- **WHEN** a request carries a note alongside a coded challenge that is not "other"
- **THEN** the request is rejected and nothing is stored

### Requirement: Questionnaire access is owner-scoped and partially updatable

The system SHALL expose the questionnaire only to the authenticated owner of the account,
through a read and a partial update. An update carries only the fields the caller is
changing, and a field the request omits MUST keep its stored value.

There is deliberately no operation that returns a stated answer to unstated. Every field
here is corrective in practice — a candidate restates a stage they have moved past, they do
not withdraw one — and no surface in the wizard produces a withdrawal. This matches the
contract the adjacent screening answers settled on for the same reason, and avoids the
presence-detection machinery that distinguishing "omitted" from "explicitly null" would
otherwise require of every caller.

Reading a questionnaire that has never been answered MUST succeed and report every field as
unstated, rather than failing as missing.

#### Scenario: Partial update leaves untouched fields alone

- **WHEN** a user who has stored a stage and a challenge submits an update carrying only a new challenge
- **THEN** the challenge changes and the stored stage is unchanged

#### Scenario: Reading before answering anything

- **WHEN** a user who has never answered reads their questionnaire
- **THEN** the read succeeds and reports every field as unstated

#### Scenario: Another account's answers are unreachable

- **WHEN** an authenticated user reads or writes the questionnaire
- **THEN** only their own account's answers are affected, and no request can name another account

### Requirement: Questionnaire answers are removed with the account

The system SHALL delete a user's questionnaire answers when their account is deleted,
leaving no orphaned row behind.

#### Scenario: Account deletion removes the answers

- **WHEN** an account with stored questionnaire answers is deleted
- **THEN** the stored answers are removed
