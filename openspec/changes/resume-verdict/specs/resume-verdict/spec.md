## ADDED Requirements

### Requirement: Deterministic market skill-gap scoring

The system SHALL compute a verdict for a search profile by comparing the profile's saved skills against the most in-demand skills on the live market for the profile's specialization(s). The market is the facet distribution of open jobs filtered to the profile's specialization categories (OR-combined). The "expected" set SHALL be the top 20 skills ordered by descending open-job count, breaking ties by ascending slug for determinism.

#### Scenario: Stack match reflects coverage of the top skills
- **WHEN** a profile's skills cover 11 of the 20 top market skills for its specializations
- **THEN** the verdict reports `stack_match` = 55 (round(11/20 × 100)) and marks those 11 rows `have: true`

#### Scenario: Fewer than twenty market skills available
- **WHEN** the market for the specializations returns only 8 distinct skills
- **THEN** the breakdown contains 8 rows and `stack_match` is computed against a denominator of 8, not 20

#### Scenario: No market data available
- **WHEN** the search/facet backend is not configured
- **THEN** the verdict endpoint responds 503 and no verdict is produced

### Requirement: Per-gap unlock percentage

For each expected skill the candidate lacks (a gap), the system SHALL report an `unlock` percentage equal to the share of the role's open postings that list that skill (round(count / total × 100)). Covered skills SHALL NOT carry an unlock value.

#### Scenario: Gap carries its market reach
- **WHEN** a gap skill appears in 340 of 1000 open postings for the role
- **THEN** that skill's row has `have: false` and `unlock: 34`

#### Scenario: Covered skill has no unlock
- **WHEN** an expected skill is present in the profile
- **THEN** its row has `have: true` and no `unlock` value

### Requirement: Must-have designation by demand share

The system SHALL mark an expected skill as a must-have when it appears in at least a fixed demand-share fraction of the role's open postings. The verdict SHALL report `must_have_total` (must-haves within the top set) and `must_have_covered` (must-haves the profile has).

#### Scenario: High-demand skill is a must-have
- **WHEN** a skill appears in 45% of the role's postings and the must-have cutoff is 40%
- **THEN** its row has `must_have: true` and it counts toward `must_have_total`

#### Scenario: Lower-demand skill is not a must-have
- **WHEN** a skill appears in 25% of the role's postings and the cutoff is 40%
- **THEN** its row has `must_have: false`

### Requirement: LLM coherence score and gap advice

When an LLM is configured, the system SHALL let the user upload a résumé (PDF or plain text) on the verdict page and, in that same request, ask the model — over the résumé text — for a coherence score in the range 0-100 (how well the claimed skills are substantiated by the Experience section) and a short piece of advice for each must-have gap. Out-of-range scores SHALL be clamped to 0-100; advice for skills not in the requested gap set SHALL be dropped; advice text SHALL be length-bounded.

#### Scenario: Résumé analyzed with the model configured
- **WHEN** a signed-in user uploads a résumé on the verdict page and the LLM is configured
- **THEN** the returned verdict includes `coherence` (0-100) and `advice` attached to must-have gap rows

#### Scenario: Model returns an out-of-range score
- **WHEN** the model returns a coherence of 150
- **THEN** the persisted/served coherence is clamped to 100

### Requirement: Résumé text is never persisted

The system SHALL use the raw résumé text only within the analysis request and MUST NOT persist or log it. Only the derived analysis — coherence score, per-gap advice, and the analysis timestamp — SHALL be stored, on the owning search profile.

#### Scenario: Only derived analysis is stored
- **WHEN** a résumé is analyzed for a profile
- **THEN** the stored `resume_analysis` contains the coherence, advice, and `analyzed_at` but no résumé text, and reopening the verdict later shows the same coherence and advice

### Requirement: Graceful degradation without the LLM

The verdict SHALL always render its deterministic core. When the LLM is unconfigured or the analysis call fails, the endpoint SHALL still return the deterministic verdict with the coherence score and advice omitted (a 200, not an error).

#### Scenario: LLM unconfigured
- **WHEN** the server has no LLM configured and a verdict is requested
- **THEN** the response is 200 with the full deterministic breakdown and no `coherence`

#### Scenario: LLM call fails during analysis
- **WHEN** a résumé is uploaded but the model errors or returns unparseable output
- **THEN** the response is 200 with the deterministic verdict and no `coherence`/`advice`, and the raw error is not surfaced to the client

### Requirement: Verdict endpoint authentication and ownership

The verdict endpoints SHALL require an authenticated session (cookie-only) and SHALL operate only on a profile owned by the caller. Requesting or analyzing another user's profile, or a missing profile, SHALL respond 404.

#### Scenario: Owner reads their verdict
- **WHEN** a signed-in user requests the verdict for a profile they own
- **THEN** the response is 200 with the verdict

#### Scenario: Non-owner is refused
- **WHEN** a signed-in user requests the verdict for a profile owned by someone else
- **THEN** the response is 404
