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

### Requirement: A full-scope key may create an employment or an atom

The system SHALL expose `POST /me/experience/employments` and `POST /me/experience/atoms`,
admitted by the same authentication the existing `GET /me/experience` accepts — a cookie or
a full-scope API key. Both SHALL run the entity's existing `Sanitize`/`Validate` before
persisting and SHALL respond `201` with `{"data": <created entity>}`. Neither SHALL require
a narrower API-key scope than `full`, since no user-facing key-minting path can produce one
today. When the caller has opted into requiring context on interactive creates,
`POST /me/experience/atoms` SHALL additionally refuse an atom whose context is empty.

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

#### Scenario: A context-gated create without context is refused

- **WHEN** a caller who has opted into requiring context posts an atom with a claim and no context
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

### Requirement: Project employments expose a name on the wire, not a company

On experience-bank HTTP surfaces that return or accept an employment, a row with `kind` equal to `project` SHALL carry its place label in the JSON field `name`. That response MUST NOT present the place label as `company`. A row with `kind` equal to `job` SHALL continue to use `company` (and optional `role`) as today.

On write (`POST` / `PUT` employments), a project MAY send `name`; a legacy `company` value for a project MUST still be accepted and stored as that same place label so existing clients keep working. Persistence MAY keep a single storage column for the label; the requirement binds the **wire** vocabulary to the kind, not the column name.

Assistant or tool projections that describe a banked place to the model or UI SHALL use the same kind-aware field names (`name` for projects, `company` for jobs).

#### Scenario: Listing a project returns name

- **WHEN** the owner lists experience and the bank holds a project whose place label is "telagon.io"
- **THEN** that employment's JSON includes `"kind":"project"` and `"name":"telagon.io"` and does not include a populated `company` for that label

#### Scenario: Listing a job still returns company

- **WHEN** the owner lists experience and the bank holds a job at RingCentral
- **THEN** that employment's JSON includes `"kind":"job"` and `"company":"RingCentral"`

#### Scenario: Creating a project with name works

- **WHEN** a caller posts an employment with `kind` project and `name` set
- **THEN** the project is persisted and the created entity is returned with `name`

#### Scenario: Creating a project with legacy company still works

- **WHEN** a caller posts an employment with `kind` project and `company` set (no `name`)
- **THEN** the place label is stored and returned as `name` on the response

#### Scenario: Experience UI labels a project by name

- **WHEN** the owner opens the experience view on a bank that includes a project
- **THEN** the project is shown under its name rather than under a “company” label

### Requirement: Unplaced achievements can be promoted to a project employment

The owner MUST be able to create a `kind=project` employment and attach an existing unplaced atom to it (create employment, then update the atom with that `employment_id`), using the same authenticated employment and atom write APIs the Experience UI uses. After promotion, the atom MUST appear under that project on bank reads and MUST leave the unplaced set. Automated tests MUST cover create-project-then-attach for an unplaced atom.

#### Scenario: Create project and attach unplaced atom

- **WHEN** an owner creates a valid project employment and updates an unplaced atom to that employment id
- **THEN** a subsequent bank read lists the atom under that project and not among unplaced achievements

#### Scenario: Invalid attach is refused

- **WHEN** an owner attempts to attach an atom to an employment they do not own
- **THEN** the update fails without moving the atom

### Requirement: Bank-through-stale on the résumé read keeps identity visible

Serving banked work history on `GET /api/v1/me/resume` while the structured stamp is stale MUST NOT produce a profile parse view that looks like a complete extract with missing identity. Banked experience MAY appear in that window; contact fields MUST follow the résumé-structured-profile pending-window rules (provisional contacts from a superseded blob when available, otherwise empty with an explicit pending signal). API-key and assistant profile tools that use the contact-free professional projection remain contact-free.

#### Scenario: Stale window still shows provisional contacts on the cookie résumé read

- **WHEN** the experience bank has roles, the structure stamp is stale, and the superseded structure holds contacts
- **THEN** the cookie résumé read used by the profile Profile tab includes those contacts alongside banked experience

#### Scenario: Contact-free professional projection is unchanged

- **WHEN** an API-key or assistant consumer reads the profile `cv` block or the contact-free professional projection during a stale window
- **THEN** contact fields remain absent from that projection

### Requirement: Bank edits offer a CV refresh with consent

After a successful experience-bank mutation in the web UI (create, update, or delete of an
employment or evidence atom, including merges), the product SHALL prompt the candidate to
refresh CV content from the current seed. The prompt SHALL make clear that agreeing
rebuilds CV body content from the experience bank and current résumé seed (same meaning as
Reset from résumé). Declining or dismissing the prompt MUST leave every CV document
unchanged. The system MUST NOT rewrite a CV solely because the bank changed.

#### Scenario: Tailor workspace — agree refreshes the open tailored CV

- **WHEN** the candidate successfully adds or updates banked experience while the tailor
  workspace is open on a tailored CV, and they confirm the refresh prompt
- **THEN** the open tailored CV is reset from the current seed (existing reset-from-résumé
  behaviour) and the workspace shows the updated document

#### Scenario: Tailor workspace — decline leaves the CV alone

- **WHEN** the candidate successfully edits the bank on the tailor workspace and declines
  the refresh prompt
- **THEN** the open tailored CV is unchanged

#### Scenario: Profile experience — agree reseeds the base CV

- **WHEN** the candidate successfully edits the bank on the profile Experience tab, they
  have a base CV, and they confirm the refresh prompt
- **THEN** the base CV body is rebuilt from the current seed and presentation (title,
  template, margins, style) is preserved

#### Scenario: No silent multi-job rewrite

- **WHEN** the candidate confirms a refresh from the profile Experience tab
- **THEN** tailored copies for other vacancies are not rewritten as part of that single
  confirmation (only the base CV is refreshed from profile context)

### Requirement: Banking an atom folds its skills into an existing search profile

Whenever an atom is created or updated in the bank, through any entry point (CV-upload
import, the `experience_add`/`experience_update` chat tools, or any future direct-add
surface), the system SHALL fold the atom's canonical skills into the owner's search
profile, if and only if the owner already has a saved profile. It SHALL NOT create a
profile that does not exist. The fold SHALL apply the same merge rule a manual claim
already applies: case-insensitive de-duplication, skipping any skill present in the
profile's `excluded_skills`, and never exceeding the profile skill cap. A failure to fold
SHALL NOT fail the atom write — the atom is durably banked regardless, and the fold is
best-effort.

#### Scenario: Atom banked via CV upload for a user with a saved profile

- **WHEN** a CV upload imports a new atom whose canonical skills include `kubernetes`, and
  the owner's saved profile does not yet list `kubernetes` and does not exclude it
- **THEN** the atom is persisted, and the owner's profile skills come to include
  `kubernetes`

#### Scenario: Atom banked via chat for a user with no saved profile

- **WHEN** the `experience_add` tool banks a new atom for a user who has never saved a
  search profile
- **THEN** the atom is persisted, and no profile is created as a side effect

#### Scenario: A skill the profile has excluded is not re-added

- **WHEN** an atom is banked whose canonical skills include a skill the owner's profile
  currently lists in `excluded_skills`
- **THEN** the atom is persisted, and that skill is NOT added to the profile's `skills`

#### Scenario: Folding a skill does not remove skills the profile already excludes elsewhere

- **WHEN** an atom is banked naming skills already present in the profile's `skills`
- **THEN** the fold is a no-op for those skills — no duplicate entries, and the profile's
  `excluded_skills` and `specializations` are unchanged

#### Scenario: A skill removed from every atom is not retroactively removed from the profile

- **WHEN** an atom is updated in a way that drops a skill it previously listed, and no
  other atom names that skill
- **THEN** the profile's `skills` still lists that skill — folding only ever adds, it never
  removes a skill on the owner's behalf

