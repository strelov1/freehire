## Purpose

The first-run flow on `/onboarding`: a visitor, signed in or not, drops a CV, answers three
one-click questions and sees their matching open jobs and ATS score. Signing up from that
screen carries the CV, the search profile and an email alert into the account.

## ADDED Requirements

### Requirement: The flow runs without an account

`/onboarding` SHALL be usable by an anonymous visitor from the first step to the results
screen without signing in. It SHALL NOT redirect an anonymous visitor to `/signin`.

#### Scenario: Anonymous visitor opens onboarding
- **WHEN** a signed-out visitor opens `/onboarding`
- **THEN** the CV drop step is shown, with no sign-in prompt

### Requirement: Anonymous CV parse

The system SHALL accept a PDF CV (or the CV's text, the same two body forms `/cv/roast` takes) from an anonymous caller and answer with what it read:
- the first name, headline, the country the residence resolves to (never the residence as text), total years of experience, up to four most recent
  employers and the key skills;
- the dictionary-derived categories and seniority;
- the CV's deterministic ATS report.

It SHALL store nothing about the CV or the caller: no bytes, no extracted fields, no
account. The structured fields come from the model; if the model is unavailable or fails,
the answer SHALL still carry the dictionary-derived skills, categories, seniority and the
ATS report, and SHALL say the structured fields are missing rather than guessing them. A
file that is not a readable PDF, or a CV with no text, SHALL be refused with a client error.

#### Scenario: Readable CV
- **WHEN** an anonymous visitor uploads a readable PDF CV
- **THEN** the answer carries the first name, headline, country, employers, years, skills, categories, seniority and an ATS score
- **AND** no CV bytes or fields are persisted

#### Scenario: Model unavailable
- **WHEN** an anonymous visitor uploads a readable PDF CV while the model call fails
- **THEN** the answer carries the dictionary skills, categories, seniority and the ATS score, and marks the structured fields as unavailable

#### Scenario: Not a PDF
- **WHEN** an anonymous visitor uploads a file that is not a readable PDF
- **THEN** the request is refused with a 4xx error and nothing is stored

### Requirement: The anonymous parse is free and bounded per IP

The anonymous CV parse SHALL run the model with no paid entitlement. It is one of the named
free-AI exceptions, and the model call SHALL go out on the service credential, never on a
user's own gateway credential. Anonymous parses SHALL be bounded per client IP to five per
24 hours; the bound is enforced at the HTTP gateway, and a refused request SHALL get HTTP
429. The flow SHALL show a refused visitor a plain message offering sign-in. Signed-in
callers SHALL NOT draw on the per-IP bound.

#### Scenario: Sixth parse from one IP in a day
- **WHEN** an anonymous visitor makes a sixth CV parse from the same IP within 24 hours
- **THEN** the request is answered with 429 and the flow says the daily limit is reached and offers sign-in

#### Scenario: No credential is minted for a guest
- **WHEN** an anonymous visitor's CV is parsed
- **THEN** the model call uses the service credential and no per-user gateway credential is read or created

### Requirement: Three quick questions

After the parse, the flow SHALL ask three questions, one at a time, in this order:
- **where**: remote, the country the CV's location resolves to (offered only when it
  resolves), anywhere, or no preference. One tap. When the CV's country resolved,
  "remote" means remote work open to that country: open worldwide, to its region, or to
  the country itself.
- **next role**: up to four roles, each paired with the CV's seniority when one was
  derived, and each showing its number of open jobs. The CV's own dictionary categories
  that have open jobs for its skills under the answers so far come first, then the
  categories with the most such jobs. A role with no open jobs SHALL NOT be offered.
  When the count is unavailable, the CV's own categories are offered. A "no preference"
  choice is always there. One tap.
- **what's in your way**: the candidate survey's challenge vocabulary (replies and
  interviews, finding jobs that fit, remote or flexible work, moving country, visa
  sponsorship, passing interviews, language requirements, a first job, just starting,
  something else with an optional note). Several answers can be picked. "Just starting"
  excludes every other answer. Continuing with none picked is allowed.

The challenge answers SHALL narrow the search in two ways only:
- visa sponsorship limits it to jobs that offer sponsorship;
- remote or flexible work limits it to remote jobs when the where answer named no place.

The features that address the picked answers ("What helps") SHALL NOT be shown in the
flow. They are kept for the signed-in product introduction page, which is not linked yet.

The flow SHALL show the answers given so far and let the visitor go back to any question.

#### Scenario: Role choices come from the CV and the catalogue
- **WHEN** the CV resolves to the categories Backend and DevOps at seniority Senior, and the catalogue holds remote jobs for its skills mostly in DevOps, Full-Stack and Backend
- **THEN** the next-role question offers "Senior Backend" and "Senior DevOps" first, then "Senior Full-Stack", each with its open-job count, plus "no preference"

#### Scenario: Visa sponsorship narrows and suggests
- **WHEN** a visitor picks "Getting visa sponsorship" in the last question
- **THEN** the matches are limited to jobs offering visa sponsorship

#### Scenario: Just starting stands alone
- **WHEN** a visitor who picked two answers then picks "Nothing yet — I'm just starting"
- **THEN** only "just starting" remains picked

#### Scenario: Location does not resolve
- **WHEN** the CV's location resolves to no country
- **THEN** the where question offers remote, anywhere and no preference only

### Requirement: Match search ranked by the CV's skills

The system SHALL offer an anonymous match search that takes a skill list and the same
filters the jobs search accepts. It SHALL answer with open jobs ranked by how well each
job's skills match the given skills, each with its skill-match percent, and with the total
count. The flow SHALL search postings from the last 7 days; when that finds none, it SHALL
widen to the last 30 days and say so. A filter the search cannot read SHALL be reported in
`meta.ignored_params`, not silently dropped.

#### Scenario: Ranked results
- **WHEN** the match search is called with skills Go and Kubernetes and the filter remote
- **THEN** remote open jobs come back ordered by skill match, each with a match percent, with the total

#### Scenario: Nothing in the last week
- **WHEN** no open job from the last 7 days matches the answers
- **THEN** the flow searches the last 30 days and the results screen names that window

### Requirement: Searching phase shows real catalogue figures

Between the questions and the results, the flow SHALL show a short searching phase naming
the catalogue it searches: open jobs, sources and ATS platforms, read from the published
catalogue snapshot. A figure that is not measured SHALL be left out rather than shown as
zero or estimated.

#### Scenario: Snapshot unavailable
- **WHEN** the catalogue snapshot has no measured open-job figure
- **THEN** the searching phase omits that figure and still proceeds to the results

### Requirement: Product introduction is parked in the product

The old tour's introduction — the tips for the stored survey answers, the AI connector,
CV tailoring and the browser extension — SHALL be available to a signed-in account on its
own page, without a navigation entry until it is placed in the product.

#### Scenario: Signed-in account opens the parked introduction
- **WHEN** a signed-in account with stored survey answers opens the introduction page
- **THEN** it shows the tips for those answers and the connector, tailoring and extension sections

### Requirement: Results screen

The results screen SHALL show:
- the parsed profile;
- the answers as chips;
- the match count and its time window;
- the employers of the top matches as a logo stack of at most ten, with a line naming two
  and counting the rest;
- the top five matching jobs, one line each: logo, title, company, work mode or country,
  age, and the match percent;
- a link to the feed with the same search;
- the ATS score as a ring with its band (Strong, Fair, Needs work), the top three fixes
  and a link to the public CV check;
- one closing row: start over, and the save action (sign-in for an anonymous visitor).

Every link to a job, the feed or the CV check SHALL open in a new tab, so the flow is not
lost. The jobs SHALL NOT be blurred or hidden behind sign-up: they are public. No figure
on the screen SHALL be invented or rounded up.

#### Scenario: Results are open
- **WHEN** an anonymous visitor reaches the results screen with matches
- **THEN** each of the top five jobs shows its title, company and match percent, and opens the job page in a new tab when clicked

### Requirement: Signing up keeps what the flow found

The results screen SHALL offer one sign-in action that takes the visitor to the site's
sign-in page and brings them back to `/onboarding` afterwards. When the visitor returns
with a NEW account, the flow SHALL, without asking again:
- store the uploaded CV as the account's CV;
- save the search profile from the answers and the CV (specialization and seniority from
  the next-role answer, the CV's skills, the where answer as location preference);
- subscribe the profile alert over email;
- save the picked challenge answers (and the note) to the candidate survey, when any were
  picked;
- mark onboarding complete;
- take the visitor to their feed.

This SHALL also work when sign-up leaves for an OAuth provider. A visitor who signs in to
an account that existed before the sign-in started (judged by the account's creation time,
not by whether it was ever onboarded) SHALL instead get an explicit save action, so nothing
lands in that account unasked. Only a browser that started the sign-in from the results
screen, within the last 30 minutes, SHALL have its CV carried. If one of the steps fails,
the account SHALL stay signed in, the other steps SHALL still run, and the visitor SHALL be
told which part did not save.

#### Scenario: Email sign-up from results
- **WHEN** an anonymous visitor on the results screen follows the sign-in action and registers with email and password
- **THEN** they return to `/onboarding`, and the new account has the uploaded CV stored, a search profile with the CV's skills and the chosen role, and a profile alert subscribed over email, and the visitor lands on the feed

#### Scenario: OAuth round trip
- **WHEN** an anonymous visitor on the results screen signs up with Google and returns from the provider
- **THEN** the same CV, profile and alert are saved to the new account without re-uploading

#### Scenario: Existing account signs in from results
- **WHEN** a visitor follows the sign-in action and signs in to an account that had already been onboarded
- **THEN** the results are shown with a save action, and nothing is saved until they use it

#### Scenario: Someone else's draft in a shared browser
- **WHEN** a visitor signs in on a browser holding another person's onboarding draft that no sign-in from the results started
- **THEN** the draft is discarded and nothing from it is shown or saved

#### Scenario: Alert fails to save
- **WHEN** the profile alert cannot be subscribed after sign-up
- **THEN** the CV and profile are still saved, the visitor is signed in and told the alert was not set up

### Requirement: Signed-in accounts run the same flow

A signed-in account whose onboarding is not complete SHALL still be routed to
`/onboarding` once, and onboarding SHALL be marked complete as soon as the flow is shown.
For a signed-in account, the results screen SHALL replace sign-up with a single save action
that does the same carrying.

#### Scenario: Account registered on the sign-in page
- **WHEN** an account registered directly on `/signin` first lands on any page
- **THEN** it is routed to `/onboarding`, and a later visit is not routed there again

### Requirement: The CV stays in the visitor's browser until sign-up

Before an account exists, the uploaded CV and the answers SHALL be kept only in the
visitor's browser. They SHALL be discarded once carried into an account or when the visitor
starts over.

#### Scenario: Starting over
- **WHEN** a visitor starts over from the results screen
- **THEN** the kept CV and answers are discarded and the drop step is shown
