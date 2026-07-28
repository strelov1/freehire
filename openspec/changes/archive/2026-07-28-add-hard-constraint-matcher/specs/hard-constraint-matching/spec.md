## ADDED Requirements

### Requirement: Deterministic hard-constraint evaluation

The system SHALL provide a pure, deterministic evaluator (`internal/hardconstraint`) that compares a job's structured requirements against a caller's structured résumé across six categories — experience-years, education, language, work-authorization, location-and-work-mode, and certification — and returns a list of typed blockers. Each blocker MUST carry a category, a severity, a numeric score-cap, a human-readable reason, an anti-hallucination action string, and a boolean `met` flag. The evaluator MUST perform no I/O and no LLM calls; it operates only on the plain structs its callers pass in, in the same dict-only discipline as `internal/jobmatch` and `internal/classify`.

#### Scenario: Unmet experience requirement produces a blocker

- **WHEN** a job requires `experience_years_min` = 5 and the caller's résumé shows `total_years` = 3
- **THEN** the evaluator returns an experience blocker with `met = false`, a reason naming both numbers, and its category's score-cap

#### Scenario: Met requirement is reported as satisfied, not a blocker

- **WHEN** a job requires a bachelor's degree and the résumé shows a master's degree
- **THEN** the evaluator returns the education entry with `met = true` and it is not counted as an unmet blocker

### Requirement: Never emit a false blocker

The evaluator SHALL evaluate a category only when BOTH the job carries the requirement AND the résumé carries the corresponding evidence field. A missing enrichment field, an absent or unparsed résumé field, or an unresolved value MUST cause the category to be skipped silently — never a blocker and never a score-cap. Language MUST default to informational only (not a blocker) because résumé language lists rarely encode a proficiency level.

#### Scenario: Missing résumé evidence skips the category

- **WHEN** a job requires 5+ years but the caller has no structured résumé (or no `total_years`)
- **THEN** the evaluator emits no experience blocker and applies no experience score-cap

#### Scenario: Job without the requirement skips the category

- **WHEN** a job's enrichment carries no `education_level`
- **THEN** the evaluator emits no education blocker regardless of the résumé

### Requirement: Severity tiers drive a score-cap ceiling

Each category SHALL carry a severity and a score-cap where a lower cap means a harder blocker: work-authorization 50, certification 60, education and experience 65, language 70, location-and-work-mode 75. When multiple blockers are unmet, the overall cap the evaluator reports MUST be the minimum score-cap over the unmet blockers. The cap is a ceiling on a downstream score, never a subtractive penalty.

#### Scenario: Hardest unmet blocker sets the ceiling

- **WHEN** a caller has an unmet certification (cap 60) and an unmet location constraint (cap 75)
- **THEN** the overall reported cap is 60

### Requirement: Shared credential vocabulary

Certification comparison SHALL normalize both the job's required certifications and the résumé's certifications through one shared curated credential vocabulary that maps aliases to canonical slugs (IT-first plus genuinely global credentials). A required certification counts as met only when a canonical slug held by the résumé equals a canonical slug required by the job.

#### Scenario: Alias on either side resolves to the same canonical slug

- **WHEN** a job requires the canonical slug for "AWS Certified Solutions Architect" and the résumé lists a known alias of it
- **THEN** the certification is reported as met

### Requirement: Degree ladder and equivalence

Education comparison SHALL rank degrees on an ordered ladder (none < ged < associate < bachelor < master < phd) after normalizing degree names through a curated equivalence dictionary, and MUST treat the requirement as met when the résumé's highest ranked degree is at or above the job's required rank.

#### Scenario: Higher degree satisfies a lower requirement

- **WHEN** a job requires an associate degree and the résumé shows a bachelor's degree
- **THEN** the education requirement is reported as met
