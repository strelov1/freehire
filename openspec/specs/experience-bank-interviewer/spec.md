# experience-bank-interviewer Specification

## Purpose
The in-bank interviewer flow, its failure and retry surfaces, and the collapsed achievements summary it selects from. Split out of experience-bank.

## Requirements

### Requirement: Entering the interviewer starts the conversation

Opening the assistant under the `profile` preset SHALL begin the interview without
requiring the candidate to compose an opening message: the entry SHALL create a new
session and send a first message on the candidate's behalf, so the agent's first response
is a question about a thin spot in their bank. When the arrival names specific atom ids,
that first question SHALL be about those atoms — enriching a thin one or reconciling
near-duplicates — rather than an unrelated gap. The message SHALL be sent only into a
session with no history, so reloading or reopening a conversation that has already started
never repeats it. Entering the assistant by any other route — the account nav, a saved
chat URL, "New chat" — SHALL send nothing and open silent.

#### Scenario: The interview opens on a question

- **WHEN** the candidate follows the experience view's action into the assistant
- **THEN** a new `profile` session is created, an opening message is recorded as theirs,
  and the agent's first reply asks about a specific gap in their bank

#### Scenario: The interview opens on a selected cluster

- **WHEN** the candidate follows the experience view's tailor action with two
  near-duplicate atom ids in the arrival
- **THEN** a new `profile` session is created, an opening message is recorded as
  theirs, and the agent's first reply asks about reconciling or enriching those
  atoms

#### Scenario: The opening message survives the move to the session's address

- **WHEN** the entry rewrites the address to the newly created session's own URL while
  that first turn is streaming
- **THEN** the turn continues to completion and its answer is rendered, rather than being
  aborted and leaving the candidate's message unanswered

#### Scenario: A conversation with history is never re-opened for them

- **WHEN** the candidate reloads a `profile` session that already holds messages
- **THEN** the stored transcript is repainted and no further opening message is sent

#### Scenario: Other entries stay silent

- **WHEN** the candidate opens the assistant from the account navigation or starts a new
  chat from the session rail
- **THEN** no message is sent on their behalf and the composer waits for them

### Requirement: An employment's achievements are collapsed behind an expandable summary

The experience view SHALL show each employment's achievement list collapsed by default,
summarized as a count ("N achievements", or a message that none are recorded yet). Expanding
an employment SHALL reveal its achievements in place, without navigating away from the
experience view or changing the page's URL. An employment SHALL start expanded when it is the
only reason the view is showing something the candidate needs to act on (see the unconfirmed
banner requirement below).

#### Scenario: A collapsed employment shows a count

- **WHEN** a signed-in user opens the experience view and an employment holds achievements
- **THEN** that employment renders collapsed, showing how many achievements it holds, and no
  achievement bullet text

#### Scenario: Expanding reveals the achievements in place

- **WHEN** the candidate activates a collapsed employment's summary
- **THEN** its achievements render beneath it in the same view, and the browser URL does not
  change

#### Scenario: An employment with no achievements says so

- **WHEN** an employment holds no achievements
- **THEN** its summary says none are recorded yet, rather than showing a count of zero

### Requirement: The unconfirmed-achievements banner links to what it is about

When the bank holds one or more achievements the assistant recorded but the candidate has not
confirmed, the banner reporting that count SHALL be an activatable control. Activating it
SHALL expand the employment (or the unplaced section) holding the first unconfirmed
achievement, in bank order, and SHALL bring that achievement into view.

#### Scenario: Activating the banner reaches the first unconfirmed achievement

- **WHEN** the bank holds unconfirmed achievements and the candidate activates the banner
- **THEN** the employment holding the first unconfirmed achievement (in bank order) expands
  if it was collapsed, and that achievement is scrolled into view

#### Scenario: An unconfirmed achievement with no employment is reachable too

- **WHEN** the first unconfirmed achievement in bank order has no employment attached
- **THEN** activating the banner reveals it in the "Not tied to a role" section

### Requirement: A failed résumé extraction is shown on the bank view with a retry action

The experience bank view SHALL read the résumé status (`GET /me/resume`'s `parse_status`/
`parse_detail`) and, when `parse_status` is `failed`, SHALL show a banner explaining that the
résumé could not be fully read and offering a retry action. The banner MUST NOT be shown when
`parse_status` is `ok` or absent (no résumé uploaded at all).

Activating the retry action SHALL call the retry endpoint and, on success, refresh the bank so
any newly-derived employments/achievements appear without a page reload. While waiting for the
outcome the view SHALL poll rather than claim an outcome it has not confirmed; if the wait is
abandoned before a terminal outcome is read, the view MUST NOT show the failed-extraction banner
on the strength of that alone — giving up asking is not the same as a confirmed failure.

#### Scenario: A failed extraction shows the banner

- **WHEN** the candidate opens the bank view and their résumé's `parse_status` is `failed`
- **THEN** a banner explains the résumé could not be fully read and offers a retry action

#### Scenario: A successful extraction shows no banner

- **WHEN** the candidate opens the bank view and their résumé's `parse_status` is `ok`
- **THEN** no résumé-extraction banner is shown

#### Scenario: No résumé uploaded shows no banner

- **WHEN** the candidate has never uploaded a résumé
- **THEN** no résumé-extraction banner is shown

#### Scenario: Retrying refreshes the bank on success

- **WHEN** the candidate activates the retry action and the retry succeeds
- **THEN** the bank reloads and shows whatever the retried extraction derived, without a page
  reload

#### Scenario: Abandoning the wait does not claim failure

- **WHEN** the candidate activates the retry action and the extraction is still pending when
  the view stops polling for it
- **THEN** the view shows no banner at all — not the failed-extraction banner — since nothing
  confirmed a failure

### Requirement: Interactive creates may require context after the owner opts in via chat

The system SHALL persist a per-user opt-in, default off, that requires a non-empty
context on interactive atom creates — `POST /me/experience/atoms` and
`experience_add`. Import, owner update, and merge MUST NOT be gated by it. The
`profile` interviewer SHALL explain the trade-off and, only after the candidate
agrees, turn the opt-in on; it SHALL also be able to turn it off the same way.
There SHALL be no visual toggle on the experience view for this opt-in. When the
opt-in is on, an interactive create with empty context SHALL be refused and SHALL
persist nothing. `get_profile` SHALL report whether the opt-in is on so the
interviewer does not ask again.

#### Scenario: Creates stay ungated until the owner agrees

- **WHEN** an owner who has never opted in records an atom with a claim and no
  context through `experience_add` or `POST /me/experience/atoms`
- **THEN** the atom is persisted

#### Scenario: After agreement, a claim-only create is refused

- **WHEN** the owner has opted in and an interactive create is submitted with a
  claim and no context
- **THEN** the write is refused, nothing is persisted, and the caller is told
  that context is required

#### Scenario: Import still banks claim-only bullets

- **WHEN** an opted-in user uploads a CV whose bullets have no situation paragraph
- **THEN** those atoms are still imported

#### Scenario: Update and merge stay ungated

- **WHEN** an opted-in user edits an atom's claim or merges two atoms whose kept
  context is empty
- **THEN** the write succeeds

#### Scenario: The interviewer can turn the opt-in off

- **WHEN** an opted-in user tells the interviewer they no longer want context
  required on new achievements
- **THEN** the opt-in is cleared and a later claim-only interactive create persists

