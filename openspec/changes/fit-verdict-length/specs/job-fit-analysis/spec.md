## ADDED Requirements

### Requirement: Recommendation has a stated length contract

The recruiter and audit stages SHALL instruct the model that `recommendation` is two or three
short prose paragraphs of hiring judgement — not a requirement-by-requirement recap of statuses the
page already shows — with no headings and no lists. The server's sanitizer length bound on
`recommendation` MUST sit well clear of that budget so it acts as a safety ceiling on runaway or
hostile model text, not as a routine shear of ordinary answers. The two numbers keep distinct jobs:
the prompt governs style; the bound governs safety.

#### Scenario: A normal-length verdict is served intact

- **WHEN** the model returns a recommendation of two or three short paragraphs within the stated budget
- **THEN** the served `recommendation` is the full text, not truncated mid-word

#### Scenario: An oversized recommendation is still bounded

- **WHEN** the model returns a recommendation longer than the sanitizer's safety ceiling
- **THEN** the stored and served value is truncated to that ceiling before persistence

#### Scenario: Both stages name the same budget

- **WHEN** Stage 2 writes a recommendation and Stage 3 rewrites it
- **THEN** both stages' prompts state the same length and shape contract for the field

## MODIFIED Requirements

### Requirement: Fuller fit-result presentation

The SPA SHALL present the fit result in fuller detail: each dimension's score and its one-line
rationale visible (not only the bar), the six dimensions including Location & work-mode fit, the
ATS requirement match, and the strengths/gaps/recommendation, in a clear visual hierarchy. The
`recommendation` (the verdict card) MUST render as multi-paragraph prose through the same
untrusted-model markup sanitizer used elsewhere on the site, so paragraph breaks and ordinary
emphasis survive without collapsing into a single block or admitting request-triggering markup. In
the narrow stacked panel the verdict card MUST use a body size that keeps a three-paragraph verdict
readable rather than a wall of large type.

#### Scenario: Dimension rationale is visible

- **WHEN** the analysis renders
- **THEN** each dimension shows its score and its rationale comment, so the user sees why, not just a number

#### Scenario: Multi-paragraph recommendation keeps its breaks

- **WHEN** the analysis carries a recommendation with two or three paragraphs separated by blank lines
- **THEN** the verdict card renders those as separate paragraphs, not one collapsed run of text

#### Scenario: Hostile markup in the recommendation never fires a request

- **WHEN** a recommendation contains a markdown image or a non-http link
- **THEN** the rendered verdict card contains no request-triggering element for that payload
