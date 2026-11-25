# experience-bank Specification

## Purpose
The durable record of what a candidate has actually done — employments and the evidence
atoms attached to them — and the surfaces that let its owner see, correct and extend it.
## Requirements
### Requirement: The bank names a way into the interviewer in every state

The experience view SHALL offer a labelled action that opens the `profile` interviewer,
and SHALL offer it whether the bank holds entries or none — the empty bank is the one that
most needs filling. The action SHALL be named for what the candidate gets rather than for
the machine that produces it, and SHALL be accompanied by a concrete example of an answer,
so the expected grain of a reply (one result, ideally carrying a number) is visible before
the conversation starts.

#### Scenario: A populated bank offers the action

- **WHEN** a signed-in user opens the experience view with achievements on record
- **THEN** an action opening the interviewer is shown alongside the count, with an example
  of the kind of achievement to describe

#### Scenario: An empty bank offers the same action

- **WHEN** a signed-in user opens the experience view with nothing recorded
- **THEN** the explanation of what the bank is for is shown together with the same action
  and example, rather than text alone

### Requirement: Entering the interviewer starts the conversation

Opening the assistant under the `profile` preset SHALL begin the interview without
requiring the candidate to compose an opening message: the entry SHALL create a new
session and send a first message on the candidate's behalf, so the agent's first response
is a question about a thin spot in their bank. The message SHALL be sent only into a
session with no history, so reloading or reopening a conversation that has already started
never repeats it. Entering the assistant by any other route — the account nav, a saved
chat URL, "New chat" — SHALL send nothing and open silent.

#### Scenario: The interview opens on a question

- **WHEN** the candidate follows the experience view's action into the assistant
- **THEN** a new `profile` session is created, an opening message is recorded as theirs,
  and the agent's first reply asks about a specific gap in their bank

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

### Requirement: A full-scope key may create an employment or an atom

The system SHALL expose `POST /me/experience/employments` and `POST /me/experience/atoms`,
admitted by the same authentication the existing `GET /me/experience` accepts — a cookie or
a full-scope API key. Both SHALL run the entity's existing `Sanitize`/`Validate` before
persisting and SHALL respond `201` with `{"data": <created entity>}`. Neither SHALL require
a narrower API-key scope than `full`, since no user-facing key-minting path can produce one
today.

#### Scenario: A full-scope key creates an employment

- **WHEN** a caller authenticated with a full-scope API key posts a valid employment
- **THEN** the employment is persisted under that caller and returned with its assigned id

#### Scenario: A full-scope key creates an atom

- **WHEN** a caller authenticated with a full-scope API key posts a valid atom
- **THEN** the atom is persisted under that caller and returned with its assigned id

#### Scenario: An invalid employment is refused

- **WHEN** a posted employment has neither a company nor a role, or names a kind outside
  the fixed vocabulary
- **THEN** the request is refused and nothing is persisted

#### Scenario: An invalid atom is refused

- **WHEN** a posted atom carries an empty claim
- **THEN** the request is refused and nothing is persisted

### Requirement: An atom created through the API is always manual provenance

Regardless of what provenance the caller sends, `POST /me/experience/atoms` SHALL persist
the atom with `manual` provenance. This is the same rule the existing owner-edit path
(`PUT /me/experience/atoms/:id`) already applies: `manual` means the owner typed this
themselves, outside a chat session the server can verify — the only provenance an HTTP
caller can honestly produce, since there is no transcript to check a `stated_in_chat` claim
against outside a chat turn.

#### Scenario: A caller-supplied provenance is discarded

- **WHEN** a posted atom's body sets `provenance` to `stated_in_chat`, `cv_import`, or
  `agent_inferred`
- **THEN** the atom is persisted with `manual` provenance regardless of the value sent

#### Scenario: A manually created atom is publishable

- **WHEN** an atom created through `POST /me/experience/atoms` is later cited by its id as
  `evidence_id` on a `cv_edit`
- **THEN** the citation is accepted, the same as any other `manual`-provenance atom

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

### Requirement: Portfolio project URL is retained on import

When a structured résumé's portfolio project is imported into the experience bank, the system SHALL persist the project's outbound URL on that employment (alongside its kind and name). An empty URL MUST remain empty; import MUST NOT invent a link. Re-import FillBlanks semantics apply: an empty banked link MAY be filled from a later extract; a non-empty banked link MUST NOT be overwritten.

#### Scenario: Project link survives import

- **WHEN** a structured résumé lists a portfolio project with a name and a non-empty URL
- **THEN** the banked project employment stores that URL

#### Scenario: Missing link stays empty

- **WHEN** a structured résumé lists a portfolio project with no URL
- **THEN** the banked project employment is created without a URL and no fabricated link is stored

#### Scenario: A user-edited link is not overwritten on re-import

- **WHEN** a banked project already has a non-empty URL and a later résumé extract carries a different URL for the matched project
- **THEN** the banked URL is unchanged

### Requirement: Banked projects are project-shaped for CV seed

A projection used to compose a CV seed SHALL expose banked `project`-kind employments as portfolio projects (name, optional URL, publishable-atom highlights), not as job rows in the work-history list. Job-kind employments remain the work-history source. An atom that is not publishable (`agent_inferred`) MUST NOT appear as a project highlight. The fit-analysis professional projection MAY continue to flatten places into a single work-history shape; this requirement binds the CV-seed composition path.

#### Scenario: A banked project carries name, link, and publishable bullets

- **WHEN** the bank holds a project employment with a URL and publishable atoms
- **THEN** the CV-seed project projection includes that name, URL, and those claims as highlights

#### Scenario: Unpublishable atoms stay off the project

- **WHEN** a banked project has only `agent_inferred` atoms
- **THEN** the CV-seed project projection includes the project identity but no highlights from those atoms

#### Scenario: Jobs are not emitted as projects

- **WHEN** the bank holds only job-kind employments
- **THEN** the CV-seed project projection is empty

### Requirement: A job entry's Company field offers catalogue suggestions while accepting free text
When adding or editing a job-kind employment, the Company field SHALL offer matching suggestions from the company catalogue as the candidate types, and SHALL still accept a company name that matches no catalogue entry.

#### Scenario: A matching company is suggested
- **WHEN** the candidate types into the Company field of the add-job or edit-job form and at least one company catalogue entry matches
- **THEN** the system shows matching companies as selectable suggestions

#### Scenario: Picking a suggestion fills the field
- **WHEN** the candidate selects a suggested company
- **THEN** the Company field is set to that company's canonical name

#### Scenario: A company outside the catalogue is still accepted
- **WHEN** the candidate types a company name that matches no catalogue entry and saves the form without picking a suggestion
- **THEN** the employment is saved with the typed text as the company name

### Requirement: A job-kind employment can be marked as current
The add-job and edit-job forms SHALL let the candidate mark a job-kind employment as current ("I currently work here"), which SHALL be mutually exclusive with an end date.

#### Scenario: Marking a job as current clears the end date
- **WHEN** the candidate checks "I currently work here" on a job-kind employment
- **THEN** the end date input is hidden and the employment is saved as current with no end date

#### Scenario: Unmarking a job as current restores the end date
- **WHEN** the candidate unchecks "I currently work here" on an employment previously marked current
- **THEN** an end date input is shown again and the employment is saved as not current

#### Scenario: A project-kind entry has no current-employment control
- **WHEN** the candidate adds or edits a project-kind entry
- **THEN** no "I currently work here" control is shown, since only job-kind employments support it

