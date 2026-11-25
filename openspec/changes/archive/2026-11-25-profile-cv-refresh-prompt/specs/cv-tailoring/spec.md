## ADDED Requirements

### Requirement: Refresh from tailor context uses reset-from-résumé

When the candidate agrees to refresh CV content after a bank edit while a tailored CV is
open in the tailor workspace, the system SHALL apply the same reset-from-résumé operation
already used by History → Reset Changes for that CV id (including base refresh as a side
effect of that endpoint, keep-if-empty summary/skills rules, and vacancy surface-align on
the tailored copy). The system MUST NOT introduce a separate silent rewrite path.

#### Scenario: Agreeing on tailor calls reset for the open CV

- **WHEN** the tailor workspace has an open tailored CV and the candidate agrees to refresh
  after a bank edit
- **THEN** `POST /api/v1/me/cvs/:id/reset-from-resume` runs for that id and the stored
  document reflects the current seed

### Requirement: Profile refresh reseeds the base CV with consent

When the candidate agrees to refresh from the profile Experience tab, the system SHALL
reseed the owner's base CV from the current seed composition (bank + current structured
résumé rules), preserving presentation. If the owner has no base CV, the system MAY create
one from the seed. The system MUST NOT require opening a tailored CV solely to refresh the
base.

#### Scenario: Base reseed endpoint (or equivalent) updates the base

- **WHEN** the owner confirms refresh on profile and a base CV exists
- **THEN** the base document body matches the current seed (with keep-if-empty for empty
  seed summary/skills) and title/template/margins/style are unchanged
