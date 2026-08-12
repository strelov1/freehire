## ADDED Requirements

### Requirement: Reset preserves skills and summary when the seed omits them

When the owner resets a tailored CV from the résumé seed, the system SHALL apply
seeded body content to the tailored document (and to the base refresh that shares the
same apply path). When the composed seed's summary is empty, the system SHALL keep the
document's existing summary. When the composed seed's skills section is empty, the
system SHALL keep the document's existing skills. The system MUST NOT copy superseded
structured summary or skills into the seed while the structured stamp is pending or
stale — keep-if-empty applies only to what is already stored on the CV.

#### Scenario: Pending extract with bank experience keeps prior skills and summary

- **WHEN** the structured stamp is not current (provisional contacts only), the experience
  bank has roles so reset is allowed, and the tailored CV already has a summary and skills
- **THEN** after reset the tailored CV still has that summary and those skills, and the
  experience sections reflect the bank seed

#### Scenario: Empty seed skills do not wipe tailored skills

- **WHEN** a current structured extract has no skills list but the tailored CV has skill chips
- **THEN** after reset the tailored CV's skill chips remain

### Requirement: Reset applies current extract skills and summary

When the composed seed carries a non-empty summary or skills (from a current structured
résumé), reset-from-résumé SHALL write those values onto the tailored document, replacing
whatever summary or skills were there. Seed wins when present.

#### Scenario: Current extract lands skills and summary on reset

- **WHEN** the structured stamp is current, the extract includes a summary and skills, and
  the owner resets a tailored CV
- **THEN** the stored tailored document's summary and skills match the seed (after any
  vacancy surface-align on the tailored copy alone)
