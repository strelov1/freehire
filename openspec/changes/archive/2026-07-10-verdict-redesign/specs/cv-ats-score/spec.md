## MODIFIED Requirements

### Requirement: Deterministic CV ATS-readiness score

The system SHALL compute an ATS-readiness score for a profile's CV from the CV's
plain text as five weighted categories whose maximum points sum to 100: **Keyword
Strength** (max 40), **Format Compliance** (max 20), **Section Completeness**
(max 15), **Content Quality** (max 15), and **Length & Density** (max 10). Each
category SHALL carry a list of line items, each with awarded-or-recoverable
`points`, a human `text`, and a `status` (pass/warn/fail). A category's `score`
SHALL be its `max` minus the points lost to warn/fail items. The report's
`overall` SHALL be the sum of the category scores, and `potential` SHALL be
`overall` plus every recoverable point across all warn/fail items, capped at 100.
The deterministic categories (Format, Section, Length, and the Keyword Strength
match) SHALL require no LLM and SHALL be reproducible (same CV text + role ⇒ same
score); the Content Quality category SHALL fall back to a deterministic proxy
(action-verb and quantified-result detection) when no LLM review is present.

#### Scenario: A scanned/image CV fails Format Compliance
- **WHEN** the extracted CV text is near-empty (a scanned or image-only PDF)
- **THEN** the machine-readable line item under Format Compliance is `fail` and `overall` is low

#### Scenario: A clean text CV scores its deterministic categories
- **WHEN** a CV has contact info, Experience/Skills sections, dates, bullets, and a normal length
- **THEN** the Format, Section, and Length categories award their points and `overall` is high

#### Scenario: Overall is the sum of categories
- **WHEN** the categories score 34, 18, 13, 11, and 9
- **THEN** `overall` = 85

#### Scenario: Potential adds back recoverable points
- **WHEN** `overall` is 72 and the warn/fail items carry 16 recoverable points
- **THEN** `potential` = 88

#### Scenario: Deterministic
- **WHEN** the same CV text and role are scored twice with no LLM review
- **THEN** the categories, line items, `overall`, and `potential` are identical

### Requirement: Role keyword-match distinct from market-coverage

The Keyword Strength category SHALL be computed from the selected role's top
in-demand skills (the role's `skills` facet) matched as literal skill-tags against
the CV TEXT (not the profile's stored skill set). The report SHALL expose the
matched skills as a `strong_keywords` list and the top unmatched role skills as a
`recommended_keywords` list, and the category score SHALL scale with the matched
share of the role's top skills. The role SHALL come from the request's facet
params (the verdict page's filter).

#### Scenario: Strong and recommended keywords split by presence in the CV text
- **WHEN** a role's top skills are {go, kubernetes, kafka} and the CV text contains "go" and "kafka" but not "kubernetes"
- **THEN** `strong_keywords` includes "go" and "kafka" and `recommended_keywords` includes "kubernetes"

#### Scenario: Keyword Strength scales with matched share
- **WHEN** the CV matches 2 of the role's 3 top skills
- **THEN** the Keyword Strength category scores about two-thirds of its 40-point max

#### Scenario: Role comes from the request filter
- **WHEN** the caller requests the report with `?category=data`
- **THEN** keyword-match uses the data role's top skills, independent of the profile's stored specializations

### Requirement: Optional LLM qualitative review, nil-safe and cached per user

When an LLM is configured, the system SHALL, on request, review the CV text for a
`content_quality` score (0-100) and a numbered `suggestions` list of concrete
improvements. The `content_quality` score SHALL set the Content Quality category
(scaled to its 15-point max), which always contributes to `overall`. When no LLM
is configured or the call fails, the endpoint SHALL return the deterministic score
only (HTTP 200) — the Content Quality category using its deterministic proxy and
no `suggestions`. The derived review SHALL be cached per user keyed to the stored
CV and reused across profiles/roles; it SHALL be invalidated when the CV is
replaced or deleted. The raw CV text SHALL NOT be persisted — only the derived
review.

#### Scenario: No LLM configured degrades cleanly
- **WHEN** the server has no LLM configured and a report is requested
- **THEN** the response is 200, Content Quality uses its deterministic proxy, and `suggestions` is empty

#### Scenario: LLM review sets Content Quality and suggestions
- **WHEN** the LLM review returns a content-quality score and improvement items
- **THEN** the Content Quality category reflects that score and `suggestions` carries the numbered items

#### Scenario: LLM review is cached and reused
- **WHEN** the LLM review has run for a user's stored CV and the report is opened again (any profile/role)
- **THEN** the cached review is served without re-calling the LLM

#### Scenario: Replacing the CV invalidates the cached review
- **WHEN** the user uploads a new CV
- **THEN** the previously cached review is cleared and not shown for the new CV

#### Scenario: Only the derived review is stored
- **WHEN** a CV is reviewed
- **THEN** the stored analysis contains the content-quality and suggestions but no CV text
