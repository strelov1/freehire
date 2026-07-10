# cv-ats-score Specification (delta)

## ADDED Requirements

### Requirement: Deterministic CV ATS-readiness score

The system SHALL compute an ATS-readiness score for a profile's CV from the CV's
plain text, using deterministic structural checks: machine-readability (near-empty
extracted text ⇒ the CV is a scan/image ⇒ hard fail), presence of contact info
(email and phone), standard sections (Experience/Education/Skills headings, EN and
RU), dates, a sane length band, and bullet usage. Each check SHALL carry a status
(pass/warn/fail), a human label, and a concrete fix. The score SHALL be
reproducible (same CV text ⇒ same score) and require no LLM.

#### Scenario: A scanned/image CV fails machine-readability
- **WHEN** the extracted CV text is near-empty (a scanned or image-only PDF)
- **THEN** the `machine_readable` check is `fail` and the overall readability is low

#### Scenario: A clean text CV scores its structural checks
- **WHEN** a CV has an email, phone, Experience/Skills sections, dates, bullets, and a normal length
- **THEN** those checks are `pass` and readability is high

#### Scenario: Deterministic
- **WHEN** the same CV text is scored twice
- **THEN** the score and checklist are identical

### Requirement: Role keyword-match distinct from market-coverage

The system SHALL report a keyword-match: of the selected role's top in-demand
skills (the role's `skills` facet), how many appear as literal skill-tag matches
in the CV text. This SHALL be computed from the CV TEXT (not the profile's stored
skill set) and SHALL name the top missing role skills in the check's fix. The role
SHALL come from the request's facet params (the verdict page's filter).

#### Scenario: Keyword-match counts role skills present in the CV text
- **WHEN** a role's top skills are {go, kubernetes, kafka} and the CV text contains "go" and "kafka" but not "kubernetes"
- **THEN** keyword-match reflects 2 of 3 and the fix names "kubernetes" as missing

#### Scenario: Role comes from the request filter
- **WHEN** the caller requests the report with `?category=data`
- **THEN** keyword-match uses the data role's top skills, independent of the profile's stored specializations

### Requirement: Optional LLM qualitative review, nil-safe and cached per user

When an LLM is configured, the system SHALL, on request, review the CV text for
qualitative issues (weak vs strong action verbs, achievement-vs-responsibility
bullets, a garbled-text flag, and concrete fixes) and blend a content-quality
score into the overall. When no LLM is configured or the call fails, the endpoint
SHALL return the deterministic score only (HTTP 200, no content-quality). The
derived review SHALL be cached per user keyed to the stored CV and reused across
profiles/roles; it SHALL be invalidated when the CV is replaced or deleted. The
raw CV text SHALL NOT be persisted — only the derived review.

#### Scenario: No LLM configured degrades cleanly
- **WHEN** the server has no LLM configured and a report is requested
- **THEN** the response is 200 with the deterministic score and no content-quality

#### Scenario: LLM review is cached and reused
- **WHEN** the LLM review has run for a user's stored CV and the report is opened again (any profile/role)
- **THEN** the cached review is served without re-calling the LLM

#### Scenario: Replacing the CV invalidates the cached review
- **WHEN** the user uploads a new CV
- **THEN** the previously cached review is cleared and not shown for the new CV

#### Scenario: Only the derived review is stored
- **WHEN** a CV is reviewed
- **THEN** the stored analysis contains the content-quality and findings but no CV text

### Requirement: ATS report endpoint authentication, ownership, and no-CV state

The ATS report endpoint SHALL require an authenticated session (cookie-only) and
operate only on a profile owned by the caller (missing/non-owned ⇒ 404). It SHALL
respond 503 when the search backend is unconfigured. When CV storage is enabled
but the caller has no CV stored, it SHALL respond 200 with a "no CV" state (so the
SPA prompts an upload) rather than an error.

#### Scenario: Owner reads their report
- **WHEN** a signed-in user requests the ATS report for a profile they own, with a CV stored
- **THEN** the response is 200 with the score and checklist

#### Scenario: Non-owner is refused
- **WHEN** a signed-in user requests the ATS report for another user's profile
- **THEN** the response is 404

#### Scenario: No CV stored
- **WHEN** the caller has CV storage enabled but no CV uploaded
- **THEN** the response is 200 with a "no CV" state, not an error
