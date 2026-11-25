## MODIFIED Requirements

### Requirement: A resolved engineering-design category vetoes deletion

A title the category dictionary places in a resolved NON-TECHNICAL CRAFT category —
`engineering_design` or `occupational_safety` — SHALL NOT be deleted
on the strength of the non-technical title dictionary — neither rejected by the
ingest catalogue filter or the liveness refresh, nor hard-deleted by either prune
rule that reads the non-technical category set (the title rule, which goes through
the shared veto, and the business rule, which reads the category set directly and
therefore needs its own exclusion). The two
vocabularies describe the same physical trades from opposite sides (the non-tech
list anchors "hvac", "sheet metal", "machinist"; the category resolves the
draughting titles those employers post), so a word match between them is not the
accidental kind the deletion veto exists to catch. A resolved category is a
deliberate placement: the posting is kept and surfaced under its facet, and only
`is_tech=false` follows from it. Non-technical categories that are not craft
categories MUST keep their current behaviour.

`occupational_safety` joins the veto for the identical reason, and the non-tech list
already proves it: that list carries "охрана труда" and "охране труда" — the Russian
name of the occupational-safety profession — so without the veto every Russian HSE
title is turned away at ingest and hard-deleted from storage, and the facet is one no
posting can reach. The veto set is stated here as the craft categories rather than as
two names so that a third craft category cannot be added to the vocabulary, by someone
with no reason to open this spec, and silently miss it.

#### Scenario: An engineering-design title survives the ingest filter

- **WHEN** a crawled board lists "HVAC Designer", whose title also matches the
  non-technical dictionary, and which carries no technical evidence
- **THEN** the posting is admitted to the catalogue, not rejected

#### Scenario: An occupational-safety title survives the ingest filter

- **WHEN** a crawled board lists "Инженер по охране труда", whose title matches the
  non-technical dictionary's "охране труда" term, and which carries no technical
  evidence
- **THEN** the posting is admitted to the catalogue, not rejected

#### Scenario: The prune title rule spares it

- **WHEN** the prune worker evaluates a stored "Sheet Metal Design Engineer" on a
  crawled board
- **THEN** the title rule does not match, so the row is not hard-deleted

#### Scenario: The prune business rule spares it too

- **WHEN** the prune worker evaluates a stored "Mechanical Design Engineer" at a
  company with no technical evidence, whose board has been retired
- **THEN** the business rule does not match — draughting is not a business role at a
  software employer, and matching would remove an engineering employer's whole
  catalogue

#### Scenario: The prune business rule spares an HSE employer too

- **WHEN** the prune worker evaluates a stored "EHS Specialist" at an oil & gas company
  with no technical evidence, whose board has been retired
- **THEN** the business rule does not match — safety work is not a business role at a
  software employer, and matching would remove that employer's whole catalogue

#### Scenario: The veto does not widen to other non-technical titles

- **WHEN** the same paths evaluate "HVAC Technician" or "Warehouse Janitorial Cleaner",
  which resolve no category
- **THEN** they are still confirmed non-technical and removed as before
