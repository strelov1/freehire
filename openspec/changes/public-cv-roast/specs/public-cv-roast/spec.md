## ADDED Requirements

### Requirement: An anonymous visitor can score a CV without an account

The API SHALL expose `POST /cv/roast`, which accepts a CV as either a PDF
(`multipart/form-data`, field `file`) or plain text (`application/json`, `{text}`) and
returns a deterministic ATS-readiness report for it. The endpoint SHALL NOT require a
session cookie or an API key.

The response SHALL carry the same `atscheck.Report` shape the signed-in ATS surfaces
return, so one frontend component renders both.

#### Scenario: A PDF from a visitor with no session scores successfully

- **WHEN** a request with no cookie and no `Authorization` header posts a text-bearing PDF
- **THEN** the response is 200
- **AND** it carries an overall score and the five scored categories

#### Scenario: Plain text is accepted on the same route

- **WHEN** an anonymous request posts `application/json` with a non-empty `text`
- **THEN** the response is 200 with the same report shape as the PDF path

#### Scenario: A non-PDF upload is refused as bad input

- **WHEN** an anonymous request posts bytes that are not a decodable PDF
- **THEN** the response is 400
- **AND** the response is not a 500

### Requirement: The uploaded CV is never stored and never sent to a model

The endpoint SHALL discard the uploaded bytes and the extracted text when the request ends.
It SHALL NOT write them to object storage, to the database, or to any cache. It SHALL NOT
invoke the LLM analyzer, so the returned report SHALL be the deterministic score alone.

#### Scenario: A roast leaves no stored résumé behind

- **WHEN** an anonymous visitor roasts a CV
- **THEN** no résumé object is written to storage
- **AND** no row recording the CV or its text is written

#### Scenario: The report is never model-refined

- **WHEN** an anonymous visitor roasts a CV while an LLM client is configured for the server
- **THEN** the returned report's Content Quality category holds the deterministic score
- **AND** the report is not marked as reviewed

### Requirement: The role is inferred from the CV, and its absence is stated

The endpoint SHALL derive the market filter from the categories `classify` resolves over the
CV's headline, taking the first as the role. When the dictionary resolves no category, the
endpoint SHALL measure coverage against the whole open catalogue and SHALL report that no
role was resolved, rather than selecting a plausible one.

The resolved seniority SHALL NOT narrow the market filter.

The response SHALL name the role it measured against, so the page can display it and offer
an override.

#### Scenario: A recognisable CV is measured against its own role

- **WHEN** a CV whose headline reads as a backend engineer is roasted
- **THEN** the response names the backend category as the role it measured
- **AND** the coverage counts are taken over that role's open postings

#### Scenario: An unrecognisable CV is measured against everything, and says so

- **WHEN** a CV whose headline resolves to no category is roasted
- **THEN** the response reports that no role was resolved
- **AND** the coverage counts are taken over the whole open catalogue

#### Scenario: A caller may override the inferred role

- **WHEN** a roast request names a role explicitly
- **THEN** that role is used as the market filter instead of the inferred one

### Requirement: The market reading names the highest-yield missing skill

Alongside the ATS score, the response SHALL report how many of the role's open postings the
CV's skills already reach, the size of the role's market, and the single missing skill that
would reach the most additional postings.

#### Scenario: A covered count and a next skill come back together

- **WHEN** a CV with resolvable skills is roasted against a role with open postings
- **THEN** the response carries the count of postings the CV's skills reach
- **AND** the count of postings in the role
- **AND** the missing skill that unlocks the most additional postings

### Requirement: The ATS half survives search being unavailable

When the facet search backend is unconfigured or fails, the endpoint SHALL still return the
ATS report and SHALL report the market reading as unavailable. It SHALL NOT return 503 in
this case.

#### Scenario: Search is down and the score still arrives

- **WHEN** a CV is roasted while the facet backend is unavailable
- **THEN** the response is 200 carrying the ATS report
- **AND** the market reading is marked unavailable rather than reported as zero

### Requirement: A CV a machine cannot read is a result, not an error

A CV yielding fewer than the readable-word minimum SHALL return a successful report in which
the machine-readability item fails, rather than an error response.

#### Scenario: A scanned CV gets the answer it came for

- **WHEN** an image-only PDF that extracts almost no text is roasted
- **THEN** the response is 200
- **AND** the machine-readability item is reported as failed

### Requirement: The endpoint is rate-limited per IP

The endpoint SHALL be rate-limited by client IP through the shared throttler, and SHALL
return 429 once a client exceeds the allowance.

#### Scenario: A scraper is cut off

- **WHEN** one IP exceeds the endpoint's hourly allowance
- **THEN** further requests from that IP return 429

### Requirement: A public page presents the roast and offers the account

The SPA SHALL serve a public route that uploads a CV to this endpoint and displays the
overall score, the five category scores with their failing items, the role it was measured
against with a control to change it, and the market reading. The page SHALL NOT require a
session to reach or to use.

The page SHALL offer the signed-in continuation — the model-written review and CV tailoring
— as its call to action.

#### Scenario: The page works with no session

- **WHEN** a visitor with no session opens the roast page and uploads a CV
- **THEN** the score, the category breakdown, and the market reading render
- **AND** no sign-in is required at any point before the result

#### Scenario: The page states the role it measured

- **WHEN** the result renders
- **THEN** the role used for the market reading is named on the page
- **AND** a control to change that role is present
