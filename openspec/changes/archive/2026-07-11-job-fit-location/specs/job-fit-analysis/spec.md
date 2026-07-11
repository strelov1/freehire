## ADDED Requirements

### Requirement: Location & work-mode fit dimension

The fit analysis SHALL score a sixth dimension, Location & work-mode fit, judging whether the
candidate can actually take the role given the job's geography (work mode, remote flag, regions,
countries, free-text location) and the caller's profile location preferences (accepted work modes,
remote reach, current base, relocation willingness). The weighted `overall_score` MUST include this
dimension with all six weights summing to 100, and title alignment and experience relevance MUST
remain the two heaviest. When the profile carries no location preferences, the dimension MUST still
resolve (scored on the job geography alone) rather than erroring.

#### Scenario: Onsite job far from a remote-only candidate

- **WHEN** the job is onsite in a country outside the candidate's base and relocation set, and the candidate prefers remote only
- **THEN** the Location & work-mode fit dimension scores low and the mismatch surfaces in the gaps/recommendation

#### Scenario: Remote job within the candidate's remote reach

- **WHEN** the job is remote and its region is within the candidate's declared remote reach
- **THEN** the Location & work-mode fit dimension scores high

#### Scenario: Profile with no location preferences

- **WHEN** a candidate with no location preferences set requests the analysis
- **THEN** the analysis still returns six dimensions (location scored on the job geography alone), never an error

### Requirement: Location signals in the prompt-chain

The prompt-chain SHALL carry the job's geography and the caller's location preferences into the
recruiter and audit stages so their reasoning and the `gaps`/`recommendation` reflect geographic and
work-mode fit, not only skills and title.

#### Scenario: Location gap explained

- **WHEN** the location dimension is a genuine mismatch
- **THEN** the recommendation names the geographic/work-mode reason rather than leaving it implicit

### Requirement: Fuller fit-result presentation

The SPA SHALL present the fit result in fuller detail: each dimension's score and its one-line
rationale visible (not only the bar), the six dimensions including Location & work-mode fit, the
ATS requirement match, and the strengths/gaps/recommendation, in a clear visual hierarchy.

#### Scenario: Dimension rationale is visible

- **WHEN** the analysis renders
- **THEN** each dimension shows its score and its rationale comment, so the user sees why, not just a number
