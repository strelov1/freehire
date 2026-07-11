## ADDED Requirements

### Requirement: Structured résumé augments the fit input

The fit prompt-chain SHALL additionally consume the caller's current structured résumé, when present, as pre-normalized candidate context supplied beside the existing CV text — never replacing it. The raw CV text remains the ground truth for requirement matching; the structured résumé is additive signal. When the caller has no current structured résumé (unconfigured LLM, not yet extracted, or stale), the chain MUST run exactly as it does today on the CV text alone, with no error.

#### Scenario: Structured résumé is provided to the chain when present

- **WHEN** the caller has a current structured résumé and requests a fit analysis
- **THEN** the fit input includes the structured résumé as pre-normalized context in addition to the CV text

#### Scenario: Analysis degrades to text-only when the structure is absent

- **WHEN** the caller has a CV but no current structured résumé
- **THEN** the fit analysis runs on the CV text alone, exactly as before, with no error
