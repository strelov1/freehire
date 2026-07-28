## ADDED Requirements

### Requirement: Public inbox feature landing page

The frontend SHALL serve a public, unauthenticated page at `/features/inbox`
that explains the inbox feature to a visitor who is not signed in. The page
SHALL render its copy server-side, so the initial HTML carries the explanatory
text without client JavaScript.

#### Scenario: Page is public and server-rendered

- **WHEN** an anonymous visitor requests `GET /features/inbox`
- **THEN** the response is 200 and its HTML body already contains the page
  headline and section copy, with no sign-in redirect

#### Scenario: Sections present

- **WHEN** the page renders
- **THEN** it contains, in order: a hero, a connect section covering both mail
  channels, a status-vocabulary section, a board-linking section, a privacy
  section, an agent-harness section, a FAQ, and a closing call to action

#### Scenario: Previews are markup, not screenshots

- **WHEN** the page renders its product previews
- **THEN** each preview is built from markup and theme tokens, and the page
  references no screenshot image file

### Requirement: Copy matches the classifier's behaviour

The page SHALL describe classification and stage advance exactly as
`internal/mailclassify` implements them, and SHALL NOT claim behaviour the code
does not have.

#### Scenario: Status vocabulary matches the code

- **WHEN** the page lists the statuses an email can be tagged with
- **THEN** the listed statuses are drawn from the `mailclassify` vocabulary —
  acknowledgement, screening, interview invitation, assessment, offer,
  rejection, info request, incomplete application — and the page states that an
  unrecognised signal is recorded as `other`

#### Scenario: Stage advance is described as forward-only

- **WHEN** the page explains how a classified email affects the tracking board
- **THEN** it states that a card only ever moves forward through
  applied → screening → responded → interview → offer, that a settled
  application is never moved automatically, and that a rejection does not move
  the card by itself

#### Scenario: Auto-linking is not overstated

- **WHEN** the page explains how an email is attached to an application
- **THEN** it states that automatic linking happens on a deterministic match
  (the mail thread, or the company name in the sender or subject) and that a
  model's pick is offered as a suggestion for the user to confirm

#### Scenario: Both mail channels are described

- **WHEN** the page renders the connect section
- **THEN** it describes the hosted freehire address as the primary way in and
  the Gmail connection as the secondary one, and states that Gmail access is
  read-only and revocable

### Requirement: Agent harness tier is documented

The page SHALL document the bring-your-own-harness tier: a caller's own client
pushes mail through `POST /me/emails` and triages it itself, and freehire never
classifies that mail.

#### Scenario: Harness section renders

- **WHEN** the page renders the agent section
- **THEN** it names the `POST /me/emails` endpoint and states that mail pushed
  this way is not classified by freehire

### Requirement: FAQ is backed by structured data

The page SHALL emit a `FAQPage` JSON-LD payload whose questions and answers are
generated from the same source as the visible FAQ text, so the structured data
can never drift from what the page shows.

#### Scenario: JSON-LD mirrors the visible FAQ

- **WHEN** the page renders
- **THEN** its `FAQPage` JSON-LD contains exactly the questions and answers
  rendered in the visible FAQ section

#### Scenario: Canonical and meta tags

- **WHEN** the page renders
- **THEN** it emits a canonical URL of `<origin>/features/inbox` together with a
  page title and description describing the inbox feature

### Requirement: The landing page is discoverable

The site SHALL link to `/features/inbox` from a dedicated "Features" group in the
footer and from the home/about pages, and SHALL list the URL in the static-pages
sitemap.

#### Scenario: Footer group

- **WHEN** any page renders its footer
- **THEN** the footer shows a "Features" group listing every feature landing,
  `/features/inbox` among them

#### Scenario: Link from the home and about pages

- **WHEN** `/` or `/about` renders
- **THEN** the "track your search" section links to `/features/inbox`, and a
  features section links to each feature landing

#### Scenario: Sitemap entry

- **WHEN** `GET /sitemap-pages.xml` is requested
- **THEN** the returned urlset contains `<origin>/features/inbox`

### Requirement: The referrals landing lives under /features

The referrals landing SHALL be served at `/features/referrals`, and its previous
address SHALL keep answering with a permanent redirect so shared links and
accumulated ranking survive the move.

#### Scenario: New address serves the page

- **WHEN** an anonymous visitor requests `GET /features/referrals`
- **THEN** the response is 200 and carries the referrals landing content with a
  canonical URL of `<origin>/features/referrals`

#### Scenario: Old address redirects

- **WHEN** a visitor or crawler requests `GET /referrals`
- **THEN** the response is a 301 to `/features/referrals`

#### Scenario: Sitemap lists only the new address

- **WHEN** `GET /sitemap-pages.xml` is requested
- **THEN** the returned urlset contains `<origin>/features/referrals` and does
  not contain `<origin>/referrals`
