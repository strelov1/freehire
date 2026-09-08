## ADDED Requirements

### Requirement: The catalogue lists members whose CV can be read

The catalogue SHALL contain exactly those accounts that are members
(`talent_network_visibility <> 'off'`) AND whose structured extract still describes the CV
currently on file (`resume_uploaded_at` is set and `resume_structured_uploaded_at` equals
it).

The second condition is not a quality bar. It is the same staleness rule the rest of the
product applies: a structure derived from a superseded CV is treated as absent, not as
stale.

#### Scenario: A member with a current extract

- **WHEN** the catalogue is requested with no filters
- **THEN** a member whose extract matches their uploaded CV appears in it

#### Scenario: A member who has just uploaded a new CV

- **WHEN** a member uploads a new CV and the extraction has not yet completed
- **THEN** the member is absent from the catalogue until the extract catches up
- **AND** their card URL answers 404 for the same period

#### Scenario: A non-member

- **WHEN** an account is `off`
- **THEN** it appears in no catalogue response regardless of filters

### Requirement: A public response carries no free text from a CV

Every string a public catalogue response emits SHALL be one of: a term resolved by a
project dictionary, a formatted date, or a fixed label owned by the application. Numbers
are emitted as numbers. No value copied from a CV's free-text fields SHALL reach a public
response.

The public projection SHALL therefore withhold: the candidate's name, photo, email, phone,
links, free-text location, headline, summary; every experience entry's employer name,
location, summary and highlights; projects entirely; and — because no dictionary reachable
from this block resolves them — languages, certifications and education.

The public projection SHALL carry: total years of experience, dictionary-resolved skills,
and per role the seniority and category its title resolves to, the period, and the
dictionary-resolved stack.

Withholding *every* employer name, not only the current one, is the point. A candidate's
stated fear is their current employer noticing, and a work history that names the previous
three employers alongside a city and a seniority identifies a person as surely as a name
does.

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

### Requirement: The catalogue is filtered on facets, not on text

The catalogue SHALL be filterable by specialization, category, seniority, skills,
timezone region, city and years of experience. Every filter SHALL take values from a
closed vocabulary or a number; the catalogue SHALL NOT accept a free-text query.

Specialization and category are different questions over the same vocabulary:
specialization is what the candidate ticked on their own profile — where they want to go
— while category is derived from their most recent job title, which is where they have
been. The listing surface SHALL offer a control for SPECIALIZATION, seniority, timezone
region and years; `categories`, `skills` and `cities` are URL and API filters with no
control in this change, because two rows over one 48-value vocabulary is 96 chips on a
screen and specialization is the one a recruiter is asking about.

There is no language filter, because there is no language data — see the withholding rule
above. A filter over a field the card does not carry would be a filter nobody could act
on.

A filter that is absent SHALL be treated identically to one that is present and empty.
Multiple values within one filter SHALL match a member carrying ANY of them; different
filters SHALL narrow together.

An unreadable filter parameter SHALL be reported in `meta.ignored_params` rather than
silently widening the answer, matching the rule the rest of this API follows.

#### Scenario: Two values in one filter

- **WHEN** the catalogue is filtered by two skills
- **THEN** a member carrying either skill appears

#### Scenario: Two different filters

- **WHEN** the catalogue is filtered by a skill and a seniority
- **THEN** only members carrying both appear

#### Scenario: An empty filter

- **WHEN** a filter is present with no values
- **THEN** the answer is identical to the same request with that filter absent

#### Scenario: A filter nobody reads

- **WHEN** a request carries a parameter the catalogue does not understand
- **THEN** the parameter is named in `meta.ignored_params`
- **AND** the answer is the same as it would have been without it

#### Scenario: Specialization and category disagree

- **WHEN** a member's most recent role resolves to `backend` and their profile declares
  `ml_ai`
- **THEN** filtering by specialization `ml_ai` returns them
- **AND** filtering by category `ml_ai` does not

#### Scenario: A member with no timezone, under a timezone filter

- **WHEN** the catalogue is filtered by timezone region and a member has no timezone
- **THEN** that member is excluded
- **AND** the surface says that the filter excludes members whose timezone is unknown

### Requirement: The order is total and paging never drops or repeats a member

The catalogue SHALL be ordered by the freshness of the member's structured extract,
descending, tie-broken by the member's handle. Paging SHALL report the total behind the
same predicate as the page.

#### Scenario: Two members sharing a timestamp

- **WHEN** two members' extracts carry the same timestamp
- **THEN** their relative order is the same on every request
- **AND** walking every page returns each of them exactly once

#### Scenario: The reported total

- **WHEN** a filtered page is requested
- **THEN** `meta.total` is the count of members matching that same filter, not the count of
  all members

### Requirement: One card is served by handle, and only while its owner is a member

A single card SHALL be addressable only by the member's catalogue handle — never by the
account's sequential id, its username, or any other identifier the account carries
elsewhere. A card whose owner is not a member, and a handle that belongs to nobody, SHALL
be answered identically, so the route cannot be used to learn whether an account exists.

Membership SHALL be re-checked against the database when a card is served, not read from
whatever snapshot the list is served from.

#### Scenario: A member's card

- **WHEN** a visitor requests a member's card by their handle
- **THEN** the card is served under the public projection

#### Scenario: A former member's card

- **WHEN** a member leaves and a visitor requests their handle
- **THEN** the response is 404 on the next request, regardless of any cached list

#### Scenario: A handle that belongs to nobody

- **WHEN** a visitor requests a card by a well-formed handle nobody holds
- **THEN** the response is 404 with the same body as a non-member's card

#### Scenario: A malformed handle

- **WHEN** a visitor requests a card by a value that cannot be a handle
- **THEN** the response is 404, not a server error

#### Scenario: The account username is not an address

- **WHEN** a visitor requests a card by an account's `username`
- **THEN** the response is 404, unless that string happens to be somebody's minted handle

### Requirement: The list is indexable, a card is not

The catalogue list page SHALL be available to search engines. An individual card SHALL
carry `noindex` and a short cache lifetime.

A card is a person. A search engine's cache of one would outlive that person's decision to
leave, which would make leaving a promise the product cannot keep.

#### Scenario: The list page

- **WHEN** a crawler fetches the catalogue list
- **THEN** the page does not forbid indexing

#### Scenario: A card page

- **WHEN** a crawler fetches an individual card
- **THEN** the page carries `noindex`

### Requirement: Both public routes are rate-limited

The catalogue list and the single-card route SHALL each be rate-limited.

The catalogue is small, complete and machine-readable, which is exactly what makes it
worth copying wholesale; a limit on the routes themselves is what stands between it and a
copy.

#### Scenario: A caller exceeding the limit

- **WHEN** an unauthenticated caller requests the catalogue faster than the limit allows
- **THEN** the excess requests are refused with 429

#### Scenario: The limit covers the real routes

- **WHEN** the routes are registered
- **THEN** a test asserts the limiter is attached to the catalogue's own paths, not to a
  group that may not contain them
