# cv-tracer-links Specification

## Purpose
TBD - created by archiving change cv-tracer-links. Update Purpose after archive.
## Requirements
### Requirement: Link tracing is opt-in per CV and off by default

A CV SHALL carry a `tracer_links_enabled` flag defaulting to false. The flag SHALL be settable
only by the CV's owner over the cookie-authenticated update endpoint. The flag MUST NOT be a
member of the patch operations available to the tailoring agent: consent to track a third
party is the candidate's to give, and an agent MUST NOT be able to give it on their behalf.

#### Scenario: A new CV has tracing off

- **WHEN** a CV is created by any path (blank, seeded from the stored résumé, or tailored)
- **THEN** its `tracer_links_enabled` is false
- **AND** rendering it produces no tracer tokens

#### Scenario: The owner enables tracing

- **WHEN** the owner sends `PUT /api/v1/me/cvs/:id/tracer-links` with `enabled` true
- **THEN** the flag is persisted and the response reports it

#### Scenario: The toggle is not part of the CV's revision history

- **WHEN** the owner enables tracing and then undoes the CV's most recent document edit
- **THEN** the flag is still set — an undo of an unrelated edit must not revoke or re-grant
  consent to track somebody

#### Scenario: The tailoring agent cannot enable tracing

- **WHEN** `PATCH /api/v1/me/cvs/:id` is called with an operation targeting
  `tracer_links_enabled`
- **THEN** the request is rejected as an unknown operation and the stored flag is unchanged

#### Scenario: A foreign CV cannot be toggled

- **WHEN** a signed-in user sends the update for a CV they do not own
- **THEN** the system responds `404` and no flag is written

### Requirement: A traced render substitutes hrefs and preserves visible text

When a CV has tracing enabled, rendering it to PDF SHALL replace the link target of every
eligible outbound link with that link's traced URL while leaving the visible link text as the
candidate wrote it. A link is eligible when it normalises to an `http` or `https` URL;
`mailto:`, `tel:`, empty values, and links to the product's own domain SHALL be left alone.
Rendering a CV with tracing disabled SHALL produce the same output as before this capability
existed.

#### Scenario: An eligible link is traced

- **WHEN** a CV with tracing enabled and a header link `github.com/jrivera` is rendered
- **THEN** the rendered document's link target is the traced URL
- **AND** the text shown to a reader is still `github.com/jrivera`

#### Scenario: Ineligible links are untouched

- **WHEN** a traced CV contains a `mailto:` address, an empty link, and a link to the
  product's own domain
- **THEN** none of them is rewritten and none produces a token

#### Scenario: Tracing disabled leaves rendering unchanged

- **WHEN** a CV with tracing disabled is rendered
- **THEN** its visual output is identical to the same CV rendered before this capability
  existed

### Requirement: Token minting is idempotent

Minting SHALL be idempotent per (CV, document position, destination URL): repeated renders of
an unchanged CV MUST reuse the same token. Changing a destination SHALL mint a new token while
every previously issued token continues to resolve. The same destination appearing at two
document positions SHALL receive two tokens, because a click in the header and a click on a
project are different events.

#### Scenario: Re-rendering reuses tokens

- **WHEN** a traced CV is rendered twice with no edit between the renders
- **THEN** both renders carry the same tokens and no new rows are created

#### Scenario: Changing a destination mints a new token

- **WHEN** the owner edits a traced link's destination and re-renders
- **THEN** a new token is minted for the new destination
- **AND** the previous token still resolves to the previous destination

#### Scenario: One destination at two positions gets two tokens

- **WHEN** the same URL appears both as a header link and as a project link on a traced CV
- **THEN** two distinct tokens are minted

### Requirement: The redirect resolves a token and records the click

The system SHALL expose an unauthenticated `GET /cv/:token` that resolves the token to its
destination, records a click, and responds `302` to that destination. The click record SHALL
carry the time, an automated-traffic flag, device type, OS family, UA family, and referrer
host. Recording SHALL be best-effort: a failure to record MUST NOT prevent the redirect. An
unknown token SHALL yield `410` with an explanatory page rather than a bare not-found.

#### Scenario: A click redirects and is counted

- **WHEN** a visitor requests `GET /cv/<token>` for a live token
- **THEN** the response is `302` to the token's destination
- **AND** one click is recorded against that token

#### Scenario: Recording failure still redirects

- **WHEN** the click cannot be written
- **THEN** the visitor is still redirected to the destination

#### Scenario: An unknown token explains itself

- **WHEN** a visitor requests a token that never existed or whose CV was deleted
- **THEN** the response is `410` with a page stating the link is no longer active

### Requirement: The redirect accepts no destination from the caller

The redirect SHALL derive its destination solely from the stored token. It MUST NOT read a
destination, or any component of one, from a query parameter, a path segment beyond the token,
or a request header, so that the endpoint cannot be used as an open redirect.

#### Scenario: A destination supplied by the caller is ignored

- **WHEN** a request adds a URL as a query parameter to a valid token's path
- **THEN** the redirect still targets the stored destination

#### Scenario: A destination supplied without a token is refused

- **WHEN** a request supplies a URL but no resolvable token
- **THEN** no redirect to that URL is issued

### Requirement: Visitor identity is a salted hash and no raw address is kept

A click record SHALL NOT store a raw IP address. Distinct-visitor counting SHALL use a keyed
hash over the address and user agent, keyed by a configured secret, so that the stored value
cannot be reversed by enumerating the address space. When no secret is configured, the flag
MUST NOT be enablable; already-issued tokens SHALL keep redirecting and record clicks with an
empty visitor hash, and the read surfaces SHALL then omit the distinct-visitor count rather
than report a wrong one.

#### Scenario: No raw address is persisted

- **WHEN** a click is recorded
- **THEN** the stored record contains no IP address in any field

#### Scenario: The same visitor is recognised

- **WHEN** two clicks arrive from the same address and user agent
- **THEN** they share a visitor hash and count as one distinct visitor

#### Scenario: Tracing cannot be enabled without the secret

- **WHEN** the secret is not configured and the owner tries to enable tracing
- **THEN** the request is refused and the flag stays false

#### Scenario: Missing secret degrades visibly

- **WHEN** the secret is removed after tokens were issued
- **THEN** clicks are still recorded and redirected
- **AND** the read surfaces report click counts without a distinct-visitor count

### Requirement: Automated traffic is flagged once, at click time

The automated-traffic flag SHALL be computed when the click is recorded and stored, never
recomputed on read, so that changing the detection rules cannot silently rewrite history. A
request using a method other than `GET` SHALL be flagged as automated. Read surfaces SHALL
exclude flagged clicks by default and offer including them.

#### Scenario: A known crawler is flagged

- **WHEN** a click arrives with a user agent matching the automated-traffic patterns
- **THEN** the stored record is flagged

#### Scenario: A HEAD request is flagged

- **WHEN** a link checker issues `HEAD /cv/<token>`
- **THEN** the stored record is flagged as automated

#### Scenario: Changing the rules does not alter stored verdicts

- **WHEN** the detection patterns change after clicks were recorded
- **THEN** the flags on the existing records are unchanged

### Requirement: The CV's own owner does not count as an opening

A click whose request carries a valid session for the CV's owner SHALL be recorded and marked
as the owner's own, and SHALL be excluded from every count and timestamp presented as somebody
having opened the CV. The redirect is served from the product's own origin, so the owner's
session cookie accompanies their click and no extra identification is needed.

#### Scenario: The candidate tests their own link

- **WHEN** the CV's owner, signed in, follows a traced link from their own downloaded PDF
- **THEN** the click is recorded as the owner's own
- **AND** the CV's click count, distinct visitors, and CV-opened marker are unchanged

#### Scenario: A signed-out visitor counts normally

- **WHEN** a visitor with no session follows the same link
- **THEN** the click counts

#### Scenario: A different signed-in user counts normally

- **WHEN** a signed-in user who does not own the CV follows the link
- **THEN** the click counts

### Requirement: Clicks expire and deleting a CV erases its tracing

Click records older than 180 days SHALL be deleted by the repository's hard-delete worker.
Deleting a CV SHALL remove its tokens and every click recorded against them.

#### Scenario: Old clicks are swept

- **WHEN** the hard-delete worker runs and click records older than 180 days exist
- **THEN** those records are deleted and newer ones are kept

#### Scenario: Deleting a CV erases its tracing

- **WHEN** the owner deletes a traced CV
- **THEN** its tokens and all clicks on them are gone
- **AND** a subsequent request for one of those tokens yields `410`

### Requirement: The owner sees per-link click counts

The system SHALL expose an owner-scoped read returning, for each of a CV's traced links, its
destination, its total clicks, its distinct visitors, and the time of the most recent click.
The response SHALL exclude clicks flagged as automated unless they are explicitly requested.
The surface SHALL present these counts as evidence that a link was opened, not as proof that a
person read the CV.

#### Scenario: Counts for a traced CV

- **WHEN** the owner reads the tracer panel for a CV with recorded clicks
- **THEN** each traced link reports its destination, click count, distinct visitors, and last
  click time

#### Scenario: Automated clicks are excluded by default

- **WHEN** a CV's clicks include flagged records
- **THEN** the default response counts only the unflagged ones
- **AND** requesting flagged records included returns the higher counts

#### Scenario: A CV with tracing off

- **WHEN** the owner reads the panel for a CV that never had tracing enabled
- **THEN** the response is an empty list rather than an error

#### Scenario: A foreign CV's counts are not readable

- **WHEN** a signed-in user reads the panel for a CV they do not own
- **THEN** the system responds `404`

### Requirement: The tracking board marks an application whose CV was opened

Where an application has a traced CV linked to its job, the board SHALL show when that CV was
last opened by a non-automated visitor, beside the application's existing state rather than in
place of it. This timestamp MUST NOT feed the application silence derivation: someone opening
a CV is not a reply, and treating it as one would clear the silence marker at the moment it
matters most.

#### Scenario: A silent application whose CV was opened shows both

- **WHEN** an application has been silent for 24 days and its CV was opened 2 days ago
- **THEN** the card shows both the silence state and the CV-opened time

#### Scenario: Opening a CV does not stop the silence clock

- **WHEN** a click is recorded against a CV tied to an application
- **THEN** the application's `last_activity_at`, `days_silent` and `silence_state` are
  unchanged

#### Scenario: No traced CV

- **WHEN** an application has no traced CV, or its traced CV has no non-automated clicks
- **THEN** the card carries no CV-opened marker

