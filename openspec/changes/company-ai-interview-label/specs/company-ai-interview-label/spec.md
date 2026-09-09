## ADDED Requirements

### Requirement: A candidate reports that an employer screened them with an AI interviewer

The system SHALL allow any authenticated user to report that a company conducts its
interviews with an AI interviewer, through `POST /api/v1/companies/:slug/process-reports`
with a body of `{ "kind": "ai_interview" }`. The company is resolved from its slug; a slug
that matches no company MUST be rejected with `404`. The request MUST be authenticated by
session cookie or API key; an unauthenticated request MUST be rejected with `401`.

`kind` is required and MUST be one of the controlled values. Today that list holds exactly
one value, `ai_interview`; any other value MUST be rejected with `400` before any database
write. The vocabulary is a code constant mirrored by a database `CHECK`, not configuration,
because it decides what the badge renders and what the search facet declares.

The report MUST NOT enter the moderation queue and MUST NOT produce work for a reviewer.
It is a statement of fact about an employer's process; the only available moderation
action — closing a posting — would answer it with something nobody asked for.

The report MUST NOT close, hide or reorder any posting.

#### Scenario: User reports an AI interview

- **WHEN** an authenticated user `POST`s `{ "kind": "ai_interview" }` to `/api/v1/companies/micro1/process-reports`
- **THEN** the system stores a report owned by that user against that company and responds `201`
- **AND** no moderation report is created

#### Scenario: Unknown kind is rejected before any write

- **WHEN** an authenticated user `POST`s a `kind` outside the controlled vocabulary
- **THEN** the system responds `400` and writes nothing

#### Scenario: Unauthenticated report is rejected

- **WHEN** a request with no valid cookie or API key `POST`s to the endpoint
- **THEN** the system responds `401` and creates no report

#### Scenario: Unknown company slug is rejected

- **WHEN** an authenticated user `POST`s to a slug that matches no company
- **THEN** the system responds `404` and creates no report

### Requirement: One report per person per company per kind

The system SHALL treat `(user, company, kind)` as a uniqueness key. A second report of the
same kind against the same company by the same user MUST be rejected with `409`. A
different user MAY report the same company at any time.

This is the abuse bound, and it is enforced by a database constraint rather than a check in
the service, so no code path can file twice.

#### Scenario: Duplicate report by the same user is rejected

- **WHEN** a user `POST`s a report for a company and kind they have already reported
- **THEN** the system responds `409` and creates no second row

#### Scenario: A different user reporting the same company is allowed

- **WHEN** a user `POST`s a report for a company another user has already reported
- **THEN** the system stores a new report and responds `201`

### Requirement: A report is withdrawn by retraction, never by deletion

The system SHALL allow a user to withdraw their own report through
`DELETE /api/v1/companies/:slug/process-reports` naming the same kind. Withdrawal MUST set
a retraction timestamp on the existing row and MUST NOT delete it.

The row survives for two reasons. It is how the signal self-heals when an employer changes
practice — a withdrawn report stops counting the moment it is withdrawn. And it preserves
the uniqueness bound, so retracting cannot be used to file repeatedly.

A retracted report MUST NOT count toward the label. A user whose report is retracted MAY
file again for the same company and kind; doing so MUST clear the retraction on the
existing row rather than create a second one.

#### Scenario: Withdrawing stops the report counting

- **WHEN** a user `DELETE`s their own report for a company
- **THEN** the row is retained with a retraction timestamp
- **AND** the company's count for that kind decreases by one

#### Scenario: Withdrawing what was never filed

- **WHEN** a user `DELETE`s a report they never filed
- **THEN** the system responds `404` and changes no count

#### Scenario: Re-filing after retraction reuses the row

- **WHEN** a user whose report is retracted `POST`s the same report again
- **THEN** the retraction is cleared on the existing row, the count increases by one, and no second row exists

### Requirement: One report is enough, and the count is always shown with the label

The system SHALL mark a company as carrying the `ai_interview` label when it has at least
one un-retracted report of that kind. There is no contributor gate.

A gate exists for `no_response` because one person's silence is an inference from an
absence and may be bad luck. "A bot conducted my interview" is an observation, and a single
observer knows it as well as forty do. Gating it would mean the first honest reporters see
nothing change and stop reporting.

Because the threshold is one, every surface that shows the label MUST also show the count.
A reader is entitled to weigh one report differently from forty, and a bare badge hides
exactly that difference. A surface that cannot show a number MUST NOT show the label.

#### Scenario: A single report raises the label

- **WHEN** the first un-retracted `ai_interview` report is filed against a company
- **THEN** that company carries the label with a count of 1

#### Scenario: Retracting the only report clears the label

- **WHEN** the only un-retracted report against a company is withdrawn
- **THEN** that company no longer carries the label

### Requirement: The count is materialised and never read without its label

The system SHALL keep the per-company count as a materialised column on `companies`,
recomputed in the same transaction as the write that changed it — the pattern
`feedback_count` and the vote counters already follow.

The count MUST NOT be recomputed by a background job. There is one writer, and a reader
MUST never observe a label without the number that qualifies it.

#### Scenario: The counter moves with the write

- **WHEN** a report is filed or retracted
- **THEN** the company's stored count reflects it in the same transaction

### Requirement: The label travels from the company onto every job surface

The system SHALL denormalize the company's label and count onto the public job view, the
way curated collections already travel from the company onto the job. Every surface that
renders a job — the list, the search hit, the job detail and the company page — MUST be
able to show the label from the payload it already receives, without a second request.

The wire shape MUST carry the count, not merely a boolean, because the count is what the
label is required to be shown with.

#### Scenario: A job of a labelled company carries the label

- **WHEN** a client reads a job whose company has two un-retracted `ai_interview` reports
- **THEN** the response carries the label and the count `2`

#### Scenario: A job of an unlabelled company carries nothing

- **WHEN** a client reads a job whose company has no un-retracted reports
- **THEN** the response omits the label rather than carrying a zero

### Requirement: Search can exclude employers carrying the label

The system SHALL accept a search filter that excludes the jobs of companies carrying the
`ai_interview` label, and the control MUST be reachable from the filter UI rather than only
from the API — a filter a person cannot get to with a mouse is not a filter.

The filterable attribute's index settings MUST be present on the live index before any
binary that queries it is served traffic. A settings patch replaces the filterable set
wholesale, so a gap turns every request naming the attribute into an index error, which
surfaces as a `500` for every caller of the affected filter and not only the one who chose
the new value.

Because the field is not part of a document's content hash, incremental index pushes do not
carry it: a full rebuild is required for the filter to see documents written before the
label existed. Until that rebuild runs the filter MUST match fewer employers, never wrong
ones.

#### Scenario: Filtering out labelled employers

- **WHEN** a search request asks to exclude AI-interview employers
- **THEN** no job belonging to a labelled company appears in the results

#### Scenario: The filter is absent by default

- **WHEN** a search request does not name the filter
- **THEN** labelled and unlabelled employers are both returned

#### Scenario: An unrebuilt index under-matches rather than mis-matching

- **WHEN** the filter runs against documents written before the label existed
- **THEN** those documents are treated as unlabelled, and no unlabelled employer is excluded

### Requirement: The label states a practice and does not judge it

The system SHALL present the label as a neutral fact — the wording naming the practice and
the count, without a warning colour, an alert icon, or language implying wrongdoing.

Many candidates prefer an AI screen; it is fast, it runs outside office hours, and it does
not react to a gap in a CV. The label exists so a candidate can recognise the practice
before they meet it, not so the platform can rule on it. A neutral label that is trusted is
worth more than a warning that is discounted.

#### Scenario: The badge names the practice and the count

- **WHEN** a labelled company's job is rendered
- **THEN** the badge reads as a statement of the practice with its report count, in neutral styling

### Requirement: The report is filed from where the candidate met it

The system SHALL offer the report in the existing job report dialog, alongside the reasons
already there, and route it as evidence rather than as a moderation report — the same split
`no_response` takes, and pinned by the same test.

The company is resolved from the job the dialog was opened on, using the company slug the
job payload already carries; the client MUST NOT need an extra request to file.

Choosing it MUST NOT create a moderation report, and MUST NOT ask the candidate for a date
or an explanation. There is nothing to elaborate: the reason is the whole claim.

#### Scenario: Choosing AI interview files evidence

- **WHEN** a user opens the report dialog on a job and chooses the AI-interview entry
- **THEN** a company report is filed for that job's company and no moderation report is created

#### Scenario: The evidence split is pinned

- **WHEN** the AI-interview entry is routed to the moderation queue by a future edit
- **THEN** a test fails naming the split
