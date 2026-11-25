## MODIFIED Requirements

### Requirement: A public response carries no free text from a CV

Every string a public catalogue response emits SHALL be one of: a term resolved by a
project dictionary, a formatted date, or a fixed label owned by the application. Numbers
are emitted as numbers. No value copied from a CV's free-text fields SHALL reach a public
response.

The public projection SHALL therefore withhold: the candidate's name, original photo,
email, phone, links, free-text location, headline, summary; every experience entry's
employer name, location, summary and highlights; projects entirely; and — because no
dictionary reachable from this block resolves them — languages, certifications and
education.

The public projection SHALL carry: total years of experience, dictionary-resolved skills,
and per role the seniority and category its title resolves to, the period, and the
dictionary-resolved stack.

Withholding *every* employer name, not only the current one, is the point. A candidate's
stated fear is their current employer noticing, and a work history that names the previous
three employers alongside a city and a seniority identifies a person as surely as a name
does.

A member's photo is an exception to "withhold entirely" rather than to the rule itself:
the original is still never served by any public route, cached or not — see "A member's
photo, when present, is served only as an irreversibly blurred derivative" for what a
public caller may read instead.

#### Scenario: A CV field that names the employer

- **WHEN** a member's CV names their employer in exactly one place — the `company`
  column, the CV summary, a role's summary or highlights, the job title, the headline, a
  project's name or highlights, the institution, the degree line, the free-text location,
  a certification, a language entry, or a skill token
- **THEN** that name appears nowhere in the marshalled card, for every one of those
  places taken separately

#### Scenario: A CV whose prose names the employer

- **WHEN** a member's CV summary reads "at <employer> I rebuilt the billing pipeline" and
  their current role's `company` field is also set
- **THEN** neither the summary nor the employer name appears anywhere in the catalogue
  response or in that member's card

#### Scenario: A job title that carries the employer

- **WHEN** a member's most recent role title reads "Backend Engineer @ <employer>"
- **THEN** the response carries the seniority and category that title resolves to, and not
  the title's own text

#### Scenario: A title no dictionary resolves

- **WHEN** a role's title resolves to neither a category nor a seniority
- **THEN** the role still appears, carrying its period and stack under a neutral label
- **AND** the role is not dropped from the work history

#### Scenario: A skill outside the dictionary

- **WHEN** a member's CV lists a skill the skill dictionary does not resolve
- **THEN** that skill is absent from the response, and the resolved ones are present

#### Scenario: A member's stored photo

- **WHEN** a member has an uploaded headshot
- **THEN** neither the catalogue list response nor a single card's JSON response carries
  any photo URL, bytes, or reference to it
- **AND** the original image bytes are not reachable through any public route

## ADDED Requirements

### Requirement: A member's photo, when present, is served only as an irreversibly blurred derivative

A member's uploaded photo, when one exists, SHALL be servable through a single dedicated
public route, addressed by the member's catalogue handle. The route SHALL serve a
derivative image produced by a strong, fixed-strength Gaussian blur applied to the stored
original — never the original bytes, and never a lightly-blurred or reversible
transformation. There is no per-member control over this: every member with a stored photo
is blurred and shown the same way, and every member without one is indistinguishable in
the response from a member the route does not recognize at all.

The blur SHALL be computed on every request, from the stored original. No blurred
derivative SHALL be persisted as a separate stored artifact — the transformation is applied
in the response path only, so there is exactly one place a member's actual likeness is
ever read from.

Membership SHALL be re-checked against the database when the photo is served, not read
from any cached catalogue snapshot — the same freshness guarantee the single-card route
already gives, and for the same reason: a member who has left must stop resolving on the
next request.

Every reason the route might not serve an image — the handle does not currently resolve to
a member, the member has never uploaded a photo, or photo storage is unavailable — SHALL
be answered identically. A caller must not be able to distinguish "not a member" from "a
member with no photo" by the shape of the response.

The route SHALL be rate-limited under the same budget as the rest of the public talent
catalogue.

#### Scenario: A member with a stored photo

- **WHEN** a visitor requests the photo route for a handle that currently resolves to a
  member with an uploaded headshot
- **THEN** the response is an image, blurred to the point that no facial feature is
  discernible
- **AND** the response is never byte-identical to the member's stored original

#### Scenario: A member with no stored photo

- **WHEN** a visitor requests the photo route for a handle that currently resolves to a
  member who has not uploaded a headshot
- **THEN** the response is the same as for a handle that resolves to no member at all

#### Scenario: A former member

- **WHEN** a member leaves and a visitor requests the photo route for their former handle
- **THEN** the response is the same as for a handle that resolves to no member at all,
  regardless of any cached list or snapshot

#### Scenario: No per-member opt-out

- **WHEN** a member has uploaded a photo and has not made any choice about this route
- **THEN** their blurred photo is servable exactly like every other photo-carrying
  member's — there is no setting that changes this per member

#### Scenario: The route is rate-limited

- **WHEN** the photo route is registered
- **THEN** it carries a rate limiter from the same budget the other public talent routes
  share
