## ADDED Requirements

### Requirement: Certifications in the structured résumé

The structured résumé shape SHALL carry a `certifications` field: a list of the credentials the résumé claims, extracted best-effort by the résumé parser. The field MUST pass the same sanitize gate as the rest of the structured shape (bounded, capped) and MUST live in the existing `resume_structured` jsonb with no new database column. A résumé parsed before this field existed reads as absent and self-heals on the next upload, exactly like the rest of the structured shape.

#### Scenario: Résumé certifications are extracted

- **WHEN** a CV lists "AWS Certified Solutions Architect" and "PMP"
- **THEN** the structured shape's `certifications` contains entries for both

#### Scenario: Older extraction without the field degrades gracefully

- **WHEN** a structured résumé was parsed before `certifications` existed
- **THEN** `certifications` reads as absent and no error occurs; it is repopulated on the next CV upload
