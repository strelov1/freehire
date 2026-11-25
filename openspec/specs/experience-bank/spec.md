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

