# experience-bank-merge Specification

## Purpose
Merging experience atoms into richer ones, near-duplicate and thin-atom visibility, and promoting unplaced achievements to a project employment. Split out of experience-bank.

## Requirements

### Requirement: Achievement selection for Merge and Tailor works across collapsed employments

Selecting an achievement for merging or for "Tailor with assistant" SHALL NOT require its
employment to be expanded, and SHALL NOT be cleared by collapsing or expanding any employment.
A selection SHALL be able to span achievements under different employments, consistent with
"Tailor with assistant" accepting achievements from more than one place.

#### Scenario: Collapsing an employment keeps its selected achievements selected

- **WHEN** the candidate has selected an achievement under an expanded employment and then
  collapses that employment
- **THEN** the achievement remains selected and is counted in the selection toolbar

#### Scenario: A cross-employment selection remains available for Tailor with assistant

- **WHEN** the candidate selects one achievement from one employment and another from a
  different employment, with both employments left collapsed afterward
- **THEN** "Tailor with assistant" is available for the two-item selection

### Requirement: The owner can merge two atoms into one richer atom

The system SHALL let the bank's owner merge exactly two of their atoms into one.
The merge SHALL run through one owner-scoped service used by both the HTTP surface
and the in-app assistant, so the two cannot disagree. The service SHALL choose
which atom to keep (the richer of the two), union their metrics and skills,
keep the richer non-empty context, leave the kept claim unchanged, and delete the
other atom. If either atom is publishable, the kept atom SHALL be publishable;
if both are `agent_inferred`, it SHALL stay `agent_inferred` until the owner
confirms it. A merge the caller does not own, a pair that is not exactly two
distinct owned atoms, or a pair attached to two different employments SHALL be
refused and SHALL persist nothing. Import MUST NOT merge. Exact `ClaimKey`
deduplication SHALL stay unchanged.

#### Scenario: Two near-paraphrases become one richer atom

- **WHEN** the owner merges two atoms about the same work — one naming live and
  batch processing, the other naming VAD filtering and model profiles — that
  share an employment or are both unplaced
- **THEN** one atom remains, carrying both atoms' distinct metrics and skills and
  the richer context, and the other atom is gone from the bank

#### Scenario: A publishable sibling makes the merge publishable

- **WHEN** the owner merges an `agent_inferred` atom with a `stated_in_chat`,
  `manual`, or `cv_import` atom
- **THEN** the kept atom is publishable and eligible to be cited as `evidence_id`

#### Scenario: Two unconfirmed readings stay unconfirmed

- **WHEN** the owner merges two `agent_inferred` atoms
- **THEN** the kept atom remains `agent_inferred` and still cannot reach a CV
  until the owner confirms it

#### Scenario: Cross-employment merge is refused

- **WHEN** the owner asks to merge two atoms attached to different employments
- **THEN** the request is refused and both atoms remain unchanged

#### Scenario: Another user's atom cannot be merged

- **WHEN** the caller names an atom they do not own as one side of a merge
- **THEN** the response reports the atom as not found and nothing is persisted

### Requirement: Merge is available over HTTP and as an assistant tool

The system SHALL expose owner merge as `POST /me/experience/atoms/merge`, admitted
by the same cookie authentication that corrects or deletes an existing atom — a
full-scope API key SHALL NOT reach it. The body SHALL name exactly two atom ids.
A successful merge SHALL respond `200` with `{"data": <kept atom>}`. The in-app
assistant SHALL be offered an `experience_merge` tool on every preset, acting as
the session owner, calling the same service, and returning structured data naming
the kept atom and the deleted id. A refused merge SHALL come back to the model as
a tool result naming what was invalid, never as a turn failure.

#### Scenario: The owner merges from the experience view

- **WHEN** a signed-in browser caller posts two owned atom ids to
  `POST /me/experience/atoms/merge`
- **THEN** the kept atom is returned and the other id is no longer listed on
  `GET /me/experience`

#### Scenario: A leaked API key cannot merge

- **WHEN** a caller authenticated only with a full-scope API key posts a merge
- **THEN** the request is refused and both atoms remain

#### Scenario: The interviewer merges after the candidate agrees

- **WHEN** the candidate, in any assistant preset, agrees that two listed
  achievements are the same work
- **THEN** the agent can merge them with `experience_merge` and the tool result
  names the kept id so later `experience_update` or `cv_edit` can address it

#### Scenario: A one-sided or malformed merge is reported to the model

- **WHEN** the agent calls `experience_merge` with one id, the same id twice, or
  an id that is not the caller's
- **THEN** the tool returns an error naming what was wrong and the turn continues

### Requirement: Near-duplicates and thin atoms are visible without broadening ClaimKey

The system SHALL detect soft-duplicate clusters among an owner's atoms by token
similarity on each atom's claim — the same meaningful-tokenisation retrieval
already uses, not embeddings and not a broader `ClaimKey`. Two atoms whose token
Jaccard similarity is at least 0.40 SHALL belong to one cluster. The system SHALL
also derive two richness flags per atom: `needs_context` when the atom has no
context, and `needs_metrics` when it has no metrics and its claim carries no
number. These signals SHALL be computed on read, not stored. `GET /me/experience`
SHALL include them so the experience view can flag near-duplicates and thin
atoms. `get_profile` SHALL report cluster id-lists and thin counts — not atom
bodies — and SHALL cap how many clusters it returns so a large bank cannot flood
the transcript. `interview_context` SHALL carry the two richness flags on each
evidence atom it already returns.

#### Scenario: Two paraphrases of the same plugin work cluster together

- **WHEN** the bank holds “Built a Chromium plugin with custom audio transcription
  pipeline using faster-whisper models, with configurable profiles for live and
  batch processing” and “Built a Chromium plugin that transcribes audio using
  faster-whisper models with configurable profiles and VAD-based filtering…”
- **THEN** `GET /me/experience` marks them as members of the same soft-duplicate
  cluster

#### Scenario: Exact ClaimKey dedup is unchanged

- **WHEN** a user uploads a CV whose bullet normalizes to a claim key the bank
  already holds
- **THEN** no second atom is created, exactly as before this change

#### Scenario: Distinct claims with shared jargon do not cluster

- **WHEN** two atoms share only stopwords or a single generic token and their
  Jaccard similarity is below 0.40
- **THEN** they are not reported as a cluster

#### Scenario: A claim-only atom is flagged thin

- **WHEN** an atom has a claim, empty context, and no metrics or digits
- **THEN** it is reported with both `needs_context` and `needs_metrics`

#### Scenario: get_profile still omits atom bodies

- **WHEN** a user with several hundred atoms, including soft-duplicate clusters
  and thin atoms, starts a session and the agent reads their profile
- **THEN** the result carries employments, counts, cluster id-lists, and thin
  totals, and no claim or context text

### Requirement: The experience view lets the owner merge and enrich selected atoms

The experience view SHALL let the owner select multiple achievements and SHALL
offer a merge action when exactly two selected atoms are a valid merge pair, and
a tailor action that opens the `profile` interviewer on the selected atoms. The
existing per-atom edit SHALL also edit context and metrics, not only the claim.
Near-duplicate and thin flags from the list response SHALL be visible on the
atoms they describe. Deletion and single-atom edit SHALL remain available.

#### Scenario: Two selected siblings can be merged in place

- **WHEN** the owner selects two atoms that the merge service would accept and
  confirms merge
- **THEN** the list refreshes to one atom carrying the unioned richness and the
  other is gone

#### Scenario: Selected atoms open the interviewer on those ids

- **WHEN** the owner selects one or more atoms and follows the tailor action
- **THEN** a `profile` session opens with those atom ids in the arrival, so the
  agent's first question is about enriching or merging that selection rather than
  an unrelated thin spot

#### Scenario: Edit can add the missing situation and numbers

- **WHEN** the owner edits an atom that has only a claim
- **THEN** they can save a context paragraph and metrics on that same edit, and
  the stored atom carries them

### Requirement: Unplaced achievements can be promoted to a project employment

The owner MUST be able to create a `kind=project` employment and attach an existing unplaced atom to it (create employment, then update the atom with that `employment_id`), using the same authenticated employment and atom write APIs the Experience UI uses. After promotion, the atom MUST appear under that project on bank reads and MUST leave the unplaced set. Automated tests MUST cover create-project-then-attach for an unplaced atom.

#### Scenario: Create project and attach unplaced atom

- **WHEN** an owner creates a valid project employment and updates an unplaced atom to that employment id
- **THEN** a subsequent bank read lists the atom under that project and not among unplaced achievements

#### Scenario: Invalid attach is refused

- **WHEN** an owner attempts to attach an atom to an employment they do not own
- **THEN** the update fails without moving the atom

