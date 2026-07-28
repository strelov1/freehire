## ADDED Requirements

### Requirement: Tailoring respects hard-constraint guardrails

The tailoring flow SHALL consume the deterministic hard-constraint blockers' anti-hallucination action strings as guardrails, so the tailored output never fabricates a credential, degree, year count, or authorization the caller has not evidenced. When a blocker is unmet, its action string MUST be surfaced to the tailoring step as an explicit "do not claim unless true" instruction.

#### Scenario: Missing certification is not fabricated

- **WHEN** a job requires a certification the caller does not hold and the caller tailors their CV for it
- **THEN** the tailoring step receives the blocker's action string and does not invent the certification in the tailored output
