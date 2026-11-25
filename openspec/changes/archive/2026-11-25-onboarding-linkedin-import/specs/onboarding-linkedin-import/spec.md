## ADDED Requirements

### Requirement: A public LinkedIn profile URL is a second entry point to the onboarding CV step

The onboarding wizard's CV step SHALL offer, alongside the existing file dropzone, a control that accepts a public LinkedIn profile URL and derives the same pre-fill values a CV extraction produces. Neither entry point SHALL be required, and the step SHALL remain skippable exactly as before.

#### Scenario: Importing by URL pre-fills the confirm step

- **WHEN** a user on the CV step submits a public LinkedIn profile URL whose headline resolves to a role, a level and at least one skill
- **THEN** the confirm step opens with that role, level and those skills pre-selected, editable before saving

#### Scenario: The URL control does not make the step required

- **WHEN** a user on the CV step activates "Skip" without uploading a file and without submitting a URL
- **THEN** the page advances to the confirm step exactly as it does today

### Requirement: Only a LinkedIn member-profile URL is accepted

The import SHALL accept only URLs whose host is `linkedin.com`, `www.linkedin.com`, or a `<cc>.linkedin.com` country subdomain, and whose path identifies a member profile (`/in/<public-id>`). Any other URL SHALL be rejected before any outbound request is made, with a message naming what was expected. A bare public id (`istrelov`) SHALL be accepted and treated as `https://www.linkedin.com/in/istrelov`.

#### Scenario: A non-LinkedIn URL is rejected without a fetch

- **WHEN** a user submits `https://example.com/in/someone`
- **THEN** the request is rejected, and no outbound HTTP request is made to any host

#### Scenario: A LinkedIn company URL is rejected

- **WHEN** a user submits `https://www.linkedin.com/company/ringcentral`
- **THEN** the request is rejected as not a member profile

#### Scenario: A country subdomain is accepted

- **WHEN** a user submits `https://br.linkedin.com/in/istrelov`
- **THEN** the URL is accepted and the profile is fetched

#### Scenario: A bare public id is accepted

- **WHEN** a user submits `istrelov`
- **THEN** the URL is accepted and resolved to the `www.linkedin.com` profile path

### Requirement: The fetch is anonymous, bounded, and SSRF-guarded

The outbound fetch SHALL carry no cookie, no `Authorization` header, and no credential of any kind, and the service SHALL NOT accept, store, or read any LinkedIn session token. It SHALL go through the platform's guarded HTTP client so that redirects to private address space cannot be followed, SHALL enforce a request timeout, and SHALL stop reading the response body past a fixed byte cap. Exceeding the cap SHALL be treated as a failed import, not as a truncated success.

#### Scenario: No credential is sent

- **WHEN** the service fetches a profile page
- **THEN** the outbound request carries no `Cookie` and no `Authorization` header

#### Scenario: An oversized response fails the import

- **WHEN** the profile page body exceeds the configured byte cap
- **THEN** the import fails with an error, and no partially-parsed values are returned

#### Scenario: A redirect to a private address is not followed

- **WHEN** the profile URL redirects to a host that resolves to private address space
- **THEN** the fetch is refused by the guarded client and the import fails

### Requirement: Masked LinkedIn values are never emitted

LinkedIn returns unavailable text as runs of asterisks that preserve the original string's length (for example a job title as `"****** ******** ********"`). The importer SHALL detect such values and discard them, and SHALL NOT emit a masked value as a name, title, company, school, or any other field, nor feed one into the dictionaries.

#### Scenario: A masked company name is dropped

- **WHEN** the `Person` node's `worksFor` contains an entry whose `name` is entirely asterisks and separators
- **THEN** that entry contributes nothing to the response, and no asterisk string appears in any returned field

#### Scenario: A masked headline yields no derived values

- **WHEN** the `Person` node's `description` is entirely asterisks and separators
- **THEN** no role, level or skills are derived from it, and the response reports that nothing was recognised

### Requirement: Role, level and skills are derived from the headline by the existing dictionaries

The importer SHALL derive the level and role from the `Person` node's unmasked `description` (the profile headline) using the seniority/category dictionary, and the skills using the skill-tagging dictionary — the same deterministic dictionaries the CV extraction uses. The importer SHALL NOT call an LLM, and SHALL NOT introduce a second vocabulary for these fields.

#### Scenario: A conventional headline resolves all three fields

- **WHEN** the headline is `Senior Backend Engineer working in TypeScript/Node.js, Go, and Python`
- **THEN** the response carries seniority `senior`, the `backend` category, and the canonical skills the dictionary resolves from that text

#### Scenario: A headline the dictionaries do not recognise resolves nothing

- **WHEN** the headline carries no recognised role, level or skill token
- **THEN** the response carries no seniority, no categories and no skills, and the import is still reported as successful

### Requirement: A location preference is derived from the profile address

The importer SHALL derive a location preference from the `Person` node's unmasked postal address using the geography dictionary, and SHALL return the countries, regions and cities it resolves. An address the dictionary cannot resolve SHALL yield no location rather than a guess.

#### Scenario: A resolvable address yields country and region

- **WHEN** the address locality is `Florianópolis, Santa Catarina, Brazil`
- **THEN** the response carries country `br` and region `latam`

#### Scenario: An unresolvable address yields nothing

- **WHEN** the address cannot be resolved by the geography dictionary
- **THEN** the response carries no location values

### Requirement: The step discloses that work history is not imported

The CV step SHALL state, wherever the URL import is offered, that LinkedIn does not release employment history to this import, and SHALL name LinkedIn's own profile-to-PDF export as the way to bring the full history in — which the step's existing file dropzone already accepts. The disclosure SHALL be visible without the user first attempting an import.

#### Scenario: The disclosure is visible before an import is attempted

- **WHEN** a user opens the CV step
- **THEN** the URL import's disclosure about missing work history and the PDF-export alternative is visible without any prior interaction

### Requirement: An import merges into staged values and does not replace them

Imported values SHALL be merged into whatever the wizard has already staged (from a prior visit's saved profile, a CV extraction, or the user's own edits), never replacing the staged set. A field the import resolves nothing for SHALL leave the staged value untouched.

#### Scenario: An import adds to skills already staged

- **WHEN** the confirm step already has two skills staged and an import resolves a third
- **THEN** all three skills are staged

#### Scenario: An import that resolves no level leaves a staged level alone

- **WHEN** a level is already staged and the imported headline resolves no seniority
- **THEN** the staged level is unchanged

### Requirement: An import is not a CV and persists nothing by itself

A URL import SHALL NOT create a CV, SHALL NOT change the account's CV-presence state, and SHALL NOT write anything to the user's profile on its own. Its values SHALL reach the server only through the wizard's existing single profile save when the user leaves the page. The onboarding redirect gate, which is CV presence, SHALL therefore still consider an account that has only ever imported by URL as having no CV.

#### Scenario: Importing does not satisfy the onboarding gate

- **WHEN** a user imports by URL, leaves `/onboarding`, and later starts a new visit
- **THEN** they are redirected to `/onboarding` again, because the account still has no CV

#### Scenario: Importing alone writes no profile

- **WHEN** a user imports by URL and then leaves the page without the confirm step holding both a role and a skill
- **THEN** no profile save request is sent, exactly as the wizard behaves today

### Requirement: A failed import leaves the wizard usable

An import that fails — an unreachable page, a non-200 response, a page carrying no `Person` node, or a body that exceeds the cap — SHALL surface a message that says what happened and SHALL leave the step exactly as it was: the file dropzone still available, the step still skippable, and nothing staged or cleared.

#### Scenario: An unreachable profile does not block the step

- **WHEN** the profile page cannot be fetched
- **THEN** an error message is shown, no staged values change, and the user can still upload a file or skip

#### Scenario: A page with no Person node reports a clear failure

- **WHEN** the fetched page parses but carries no `Person` node
- **THEN** the import reports that the profile could not be read, and the step remains usable

### Requirement: The import endpoint is authenticated and rate limited

`POST /api/v1/me/linkedin/import` SHALL require an authenticated session and SHALL be rate limited per account, because each call causes an outbound fetch of a third-party page. An anonymous caller SHALL be refused before any outbound request is made.

#### Scenario: An anonymous caller is refused without a fetch

- **WHEN** an unauthenticated client posts a profile URL to the endpoint
- **THEN** the request is refused with an authentication error, and no outbound HTTP request is made

#### Scenario: Repeated imports are throttled

- **WHEN** one account exceeds the configured import rate
- **THEN** further imports are refused until the window passes, and no outbound HTTP request is made for the refused calls
