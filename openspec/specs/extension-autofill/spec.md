# extension-autofill Specification

## Purpose

The contact block the browser extension writes into application forms: which fields it
carries, which sources answer for them and in what order, and what an absent source yields.

It covers the candidate's identity — name, address, phone, place and links — plus their own
screening answers (authorized countries, visa sponsorship, desired salary, notice period,
willingness to relocate, 18+), which ride alongside the identity fields without a
multi-source precedence of their own. The substance of an application (summary, work
history, skills) is not here, and neither is the CV file itself: the extension has no upload
primitive, so the document is downloaded by hand from the CV surface.

Not to be confused with `cv-autofill`, which pre-fills the onboarding wizard and the profile
form from an uploaded résumé.
## Requirements
### Requirement: The autofill profile is one canonical contact block

The system SHALL serve the browser extension a fixed set of contact fields assembled
server-side, so the extension holds no rule about where a value comes from: `full_name`,
`first_name`, `last_name`, `email`, `phone`, `location`, `linkedin`, `github` and
`portfolio`. `first_name` and `last_name` SHALL be derived by splitting the full name on
whitespace — the first token is the given name, the remainder the family name. The links
SHALL be sorted by host: the first LinkedIn URL, the first GitHub URL, and the first
remaining URL as the portfolio. A field the sources do not state SHALL be empty rather
than guessed.

This block carries the candidate's identity only. It carries no summary, no work history
and no skills, because it exists to fill the contact section of an application form.

#### Scenario: The block is served with the fields the sources state

- **WHEN** an authenticated caller reads the autofill profile and a source states a name,
  a phone and a LinkedIn URL
- **THEN** the response carries those values, the given and family names split from the
  full name, and empty strings for the fields no source stated

#### Scenario: Links are sorted by host, not by order

- **WHEN** the chosen source states a personal site, then a GitHub URL, then a LinkedIn URL
- **THEN** `linkedin` is the LinkedIn URL, `github` is the GitHub URL, and `portfolio` is
  the personal site

### Requirement: The contact block resolves from an ordered list of sources

The system SHALL resolve the contact block from the first source that answers, in this
order:

1. the user's **base CV** header — the CV that is not a tailored copy;
2. the user's **structured résumé** contact fields;
3. **no source**, leaving every field empty.

A source answers when it exists and states at least one contact value; a source that
exists but states none is passed over, so an empty CV does not silence a résumé that has
the values. The chosen source answers for the **whole** block: fields SHALL NOT be merged
across sources. Merging would restore a value the candidate deliberately removed from
their CV, and the candidate's own copy is the one that must win outright.

The base CV ranks first because it is the copy the candidate authored: the structured
résumé seeds it, and the candidate corrects it afterwards. It is also the only copy the
tailoring agent cannot rewrite — the edit policy denies the agent the header's name, email,
phone and links.

Tailored CVs SHALL NOT be a source at any tier. A tailored copy is written for one vacancy,
and the autofill profile carries no vacancy.

#### Scenario: The base CV answers when it states contacts

- **WHEN** a user has a base CV whose header states their name, and also has a structured
  résumé stating a different name
- **THEN** the block carries the base CV's name

#### Scenario: The structured résumé answers when there is no base CV

- **WHEN** a user has a current structured résumé and no base CV
- **THEN** the block carries the structured résumé's contact fields

#### Scenario: An empty base CV does not silence the structured résumé

- **WHEN** a user has a base CV whose header states no contact value at all, and a
  structured résumé that states their name and phone
- **THEN** the block carries the structured résumé's name and phone

#### Scenario: Sources are not merged

- **WHEN** a user's base CV header states a name but no phone, while their structured
  résumé states both
- **THEN** the block carries the base CV's name and an empty phone

#### Scenario: A tailored CV is never a source

- **WHEN** a user's only CVs are tailored copies
- **THEN** the tailored copies are ignored and the structured résumé answers instead

### Requirement: The account email backstops an unstated address

The system SHALL fall back to the account's own email address whenever the resolved
contact block states no email — including when no source answered at all. Every other
field stays empty in that case.

An account address is a verified fact about the caller that needs no CV, so a user who has
uploaded nothing still gets the one field an application form always asks for.

#### Scenario: A user with no CV and no résumé still gets their address

- **WHEN** an authenticated caller has neither a CV nor a structured résumé
- **THEN** the block carries the account email and every other field is empty

#### Scenario: A source stating no email falls back to the account address

- **WHEN** the resolved source states a name and a phone but no email
- **THEN** the block carries that name and phone together with the account email

### Requirement: Both autofill entry points share one assembly

The system SHALL assemble the contact block once and serve it to both the extension's
deterministic read (`GET /api/v1/me/autofill-profile`) and the agent-driven fill
(`POST /api/v1/me/autofill/run`), so the values a person sees in a form and the values the
agent grounds its plan in cannot diverge. The read endpoint remains authenticated by
either the website's cookie or a full-scope API key, same as any other `mw.key` route.
The agent-driven fill does not: see "The agent-driven fill is confined to the
extension's own connection" below for what authenticates it.

#### Scenario: The agent fills from the same block the endpoint serves

- **WHEN** the agent-driven autofill runs for a user
- **THEN** it grounds its plan in the same contact block the read endpoint would serve that
  user

### Requirement: Autofill reads through the domain stores

The system SHALL read the base CV through the CV store and the account and structured
résumé through generated queries, and SHALL NOT issue hand-written SQL for them. A
hand-written statement is invisible to the query generator, so a column renamed in a
migration breaks autofill when a user opens a form rather than when the project is built.

#### Scenario: A renamed column fails the build

- **WHEN** a column the autofill block reads is renamed in a migration and the queries are
  regenerated
- **THEN** the project fails to build, rather than compiling and failing at request time

### Requirement: The autofill profile also carries the candidate's screening answers

The autofill profile block SHALL, in addition to the identity contact fields, carry the
candidate's own screening answers (`internal/screeninganswers`): `authorized_countries`,
`visa_sponsorship_needed`, `desired_salary`, `notice_period`, `willing_to_relocate` and
`age_18_or_older`. A field the candidate has not stated SHALL be empty rather than guessed,
on the same terms as an unstated identity field.

These fields are independent of the identity contact block: there is exactly one
screening-answers store, so there is no multi-source precedence to resolve for them the way
there is for name/email/phone.

#### Scenario: Screening answers ride alongside the identity fields

- **WHEN** an authenticated caller reads the autofill profile and has stated their desired
  salary and their willingness to relocate, but no other screening answer
- **THEN** the response carries those two values and empty strings for the other four
  screening fields, alongside whatever identity fields the CV/résumé/account state

### Requirement: The deterministic fallback filler answers screening questions

When the browser extension's deterministic fallback filler is in use — the path taken when
the agent-driven autofill is unavailable — it SHALL recognize application-form questions
asking about work-authorized countries, visa sponsorship, desired salary, notice period,
willingness to relocate, and age (18+), and fill them from the caller's screening answers
using the same label-matching mechanism it already uses for identity fields. A question the
caller has not answered in their profile SHALL be left blank rather than filled with a
guess, matching the existing rule for identity fields.

A checkbox-group question naming several countries SHALL be answered by the country names
its options carry, not by the raw ISO codes the profile states — the group's options are
matched against the value option-by-option, so a code that names no option would leave the
group unanswered even when the candidate has stated an answer.

#### Scenario: A notice-period question is filled from the profile

- **WHEN** the deterministic filler is planning fills for a form asking "What is your notice
  period?" and the caller's screening answers state a 30-day notice period
- **THEN** the plan fills that question with "30 days"

#### Scenario: An unanswered screening question is left blank

- **WHEN** the deterministic filler is planning fills for a form asking about desired salary
  and the caller has not stated one
- **THEN** the plan leaves that question unfilled

#### Scenario: An authorized-countries checkbox group is filled by country name, not by code

- **WHEN** a form asks "Which countries are you authorized to work in?" as a checkbox group
  offering full country names, and the caller's screening answers state authorization in the
  US and Germany
- **THEN** the plan checks the "United States" and "Germany" options, not a literal "US, DE"
  that would match neither

### Requirement: A boolean work-authorization question is not answered from the country list

A plain Yes/No "are you authorized to work [here]?" question SHALL NOT be matched to the
authorized-countries screening answer. That answer is a list of country codes, not a
boolean, and the profile carries no boolean field stating plain work authorization — filling
the question would put the wrong shape of value into a Yes/No control.

This does not apply to a question that itself asks *which* countries the candidate is
authorized to work in (a list question), which the previous requirement covers.

#### Scenario: A boolean work-authorization question stays unmatched

- **WHEN** an application form asks "Are you authorized to work in this country?" as a
  Yes/No question
- **THEN** the deterministic filler does not match it to any screening answer and leaves it
  for the caller

### Requirement: The agent-driven fill is confined to the extension's own connection

`POST /api/v1/me/autofill/run` SHALL refuse a request that authenticated by the
website's session cookie or by an API key, even though both otherwise pass this
route's ordinary auth gate. It attaches to the caller's browser-tool channel
(`internal/browsertools.Hub`, keyed by user id, not session id) and WRITES into
whatever form the browser currently attached to that channel is showing — unlike a
read, there is no safe degraded behavior, so a request that did not authenticate with
the extension's own Bearer session JWT is refused outright rather than run against a
browser the caller on this surface never opened.

#### Scenario: A cookie-authenticated request is refused

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by the website's session cookie
- **THEN** the request is refused and no browser-tool call is made

#### Scenario: An API-key-authenticated request is refused

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by a full-scope API key
- **THEN** the request is refused and no browser-tool call is made

#### Scenario: The extension's own Bearer session JWT is admitted

- **WHEN** `POST /api/v1/me/autofill/run` is called authenticated by a Bearer session JWT
- **THEN** the request proceeds to read the caller's browser-tool channel

