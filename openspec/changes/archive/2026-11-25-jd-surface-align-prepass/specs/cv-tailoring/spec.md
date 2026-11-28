## ADDED Requirements

### Requirement: New tailored copies are surface-aligned to the vacancy

When the system creates a new tailored CV for a vacancy, it SHALL apply the
jd-surface-align rewrite to that copy against the vacancy's description before
the copy is stored and before any model turn on the copy. A bootstrap that
returns an already-existing tailored CV SHALL NOT re-apply alignment merely
because bootstrap was repeated.

#### Scenario: Fresh copy uses the vacancy's skill wording

- **WHEN** bootstrap creates a tailored copy and the base CV's skills or prose use
  an alias of a skill the vacancy names under a different surface form
- **THEN** the tailored copy is stored with the vacancy's preferred surface forms
  already applied

#### Scenario: Reload does not fight user edits

- **WHEN** bootstrap is repeated for a vacancy that already has a tailored copy
- **THEN** alignment is not re-run as part of that repeat

### Requirement: Reset-from-résumé re-aligns the rebuilt copy

When a tailored CV is reset from the résumé seed, the system SHALL apply the
jd-surface-align rewrite to the rebuilt document against the bound vacancy
before the reset is considered complete. The rewrite SHALL be committed through
the same editor path as other document updates.

#### Scenario: Reset copy speaks the vacancy's wording

- **WHEN** the owner resets a tailored CV whose seed again says `IaC` and the
  vacancy prefers `infrastructure as code`
- **THEN** the stored document after reset uses the vacancy's preferred surface

### Requirement: The tailoring agent is told wording is already aligned

The tailoring system prompt SHALL tell the model that skill surface forms already
match the vacancy and that it MUST NOT rename skills for wording.

#### Scenario: Prompt forbids synonym hunting

- **WHEN** a tailoring session starts
- **THEN** the system prompt includes an instruction that wording alignment is
  done and must not be repeated
