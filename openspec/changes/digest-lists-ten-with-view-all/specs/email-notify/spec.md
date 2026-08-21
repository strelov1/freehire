## MODIFIED Requirements

### Requirement: Render a subscription digest to an email

The system SHALL render a filter-subscription digest into an email with a subject
naming the saved search and its new-match count, an HTML body, and a plain-text
alternative. Each listed job SHALL link to its on-platform freehire job page
(`<origin>/jobs/<slug>`), never to a source URL. The body SHALL list at most ten
jobs with the remainder summarized as an "and N more" tail, so a large digest reads
as a notification rather than as a page. The subject's count SHALL remain the true
match count, not the number listed. All job-, company-, salary-, and
saved-search-name text SHALL be HTML-escaped in the HTML body because it is user- or
source-derived.

The "and N more" tail SHALL link to the digest's own matched-jobs page,
`<origin>/my/notifications/<id>/jobs`, where the full match set is recorded — not to
the notification section's landing page. When the digest carries no notification id
(its in-app recording failed), the tail SHALL fall back to `<origin>/my/notifications`
so the mail is never sent without a destination.

#### Scenario: Digest renders subject, HTML, and text

- **WHEN** a digest of matched jobs is rendered for email
- **THEN** the email has a subject naming the saved search and match count, an HTML body listing each job as a link to its freehire job page, and a plain-text alternative carrying the same information

#### Scenario: Oversized digest is capped with a summary tail

- **WHEN** a digest of 67 matched jobs is rendered for email
- **THEN** the HTML and text bodies each list 10 jobs followed by an "and 57 more" summary rather than every job, and the subject still names 67

#### Scenario: The tail links to the digest's matched-jobs page

- **WHEN** a digest carrying notification id 42 is rendered with jobs omitted from the listing
- **THEN** the tail in both the HTML and text bodies links to `<origin>/my/notifications/42/jobs`

#### Scenario: A digest with no notification id still renders a tail

- **WHEN** a digest whose in-app recording failed is rendered with jobs omitted from the listing
- **THEN** the tail links to `<origin>/my/notifications` and the email is otherwise unchanged

#### Scenario: User and source text is escaped

- **WHEN** a job title, company, or saved-search name contains HTML-significant characters
- **THEN** the HTML body escapes them so the content cannot inject markup
