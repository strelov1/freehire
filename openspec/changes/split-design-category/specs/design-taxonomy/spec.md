## ADDED Requirements

### Requirement: Engineering design is a category of its own, distinct from product design

The system SHALL resolve engineering draughting and design work — mechanical,
electrical, civil/structural, and the architectural/BIM family — to a dedicated
`engineering_design` category, and SHALL reserve the `design` category for
product, visual, and experience design. Silicon and board design is NOT part of
either: it resolves to the existing `hardware` category, which already owns the rest
of that team. The engineering aliases MUST be ordered ahead of the bare `design`
alias in the title dictionary, so a qualified title resolves to the engineering
category rather than to `design` by virtue of containing the word "design".
`engineering_design` MUST be a member of the non-technical category set, so it is
surfaced as a facet but consumes no LLM enrichment or embedding budget.

#### Scenario: A qualified engineering-design title leaves the design category

- **WHEN** a job titled "Senior Mechanical Design Engineer" is classified
- **THEN** its category is `engineering_design`, not `design`

#### Scenario: Chip and board design resolve to hardware, not draughting

- **WHEN** a job titled "Physical Design Engineer", "VLSI Design Engineer" or
  "PCB Design Engineer" is classified
- **THEN** its category is `hardware` — the same category "Hardware Design Engineer"
  and "FPGA Design Engineer" already resolve to, so one silicon discipline is not
  split across two facets and keeps its technical treatment

#### Scenario: Product design keeps the design category

- **WHEN** a job titled "Senior Product Designer" or "UX Designer" is classified
- **THEN** its category is `design`

#### Scenario: The engineering category is non-technical

- **WHEN** a job resolves to `engineering_design`
- **THEN** it is not enqueued for AI enrichment or semantic embedding, and its
  derived `is_tech` is `false`

### Requirement: The bare "design engineer" title resolves to engineering design

The system SHALL resolve an unqualified "Design Engineer" title to
`engineering_design`, because that population is overwhelmingly mechanical and
industrial in the catalogue. A product-engineering hybrid SHALL be recognized only
through an explicit marker in the title — `product design engineer`,
`design systems engineer`, `design engineer, product`, or a `UI/UX engineer` form
— and those markers MUST be ordered ahead of the bare `design engineer` alias so
the more specific title wins.

#### Scenario: Unqualified title goes to engineering

- **WHEN** a job titled "Design Engineer" is classified
- **THEN** its category is `engineering_design`

#### Scenario: Explicit product marker wins

- **WHEN** a job titled "Product Design Engineer" or "Design Systems Engineer" is classified
- **THEN** its category is `design`

### Requirement: A title whose "design" names no craft resolves to no category

The system SHALL emit no category for a title where a category alias appears but
states no category of its own — "Software Design Engineer" is software engineering,
where "design" qualifies what is engineered. Such phrases SHALL be masked before the
category match, the same treatment a grade-blind phrase gets before the seniority
match, so the category comes back empty rather than being routed to draughting. The
tech-title detector SHALL still recognize the software forms, so `is_tech` stays
`true` even though the sub-category is unresolved. A title that DOES have a better
category SHALL be routed to it rather than masked.

#### Scenario: A software title keeps no category but stays technical

- **WHEN** a job titled "Senior Software Design Engineer" is classified
- **THEN** its category is empty, and its derived `is_tech` is `true`

#### Scenario: A title with a better category is routed, not masked

- **WHEN** a job titled "Cloud Design Engineer" or "Solution Design Engineer" is classified
- **THEN** its category is `devops` and `solutions_engineering` respectively

#### Scenario: Design disciplines of their own stay on the product side

- **WHEN** a job titled "Service Design Engineer", "Sound Design Engineer" or
  "Game Design Engineer" is classified
- **THEN** its category is `design`

### Requirement: A resolved engineering-design category vetoes deletion

A title the category dictionary places in `engineering_design` SHALL NOT be deleted
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
`is_tech=false` follows from it. Non-technical categories other than
`engineering_design` MUST keep their current behaviour.

#### Scenario: An engineering-design title survives the ingest filter

- **WHEN** a crawled board lists "HVAC Designer", whose title also matches the
  non-technical dictionary, and which carries no technical evidence
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

#### Scenario: The veto does not widen to other non-technical titles

- **WHEN** the same paths evaluate "HVAC Technician" or "Warehouse Janitorial Cleaner",
  which resolve no category
- **THEN** they are still confirmed non-technical and removed as before

### Requirement: Named roles cover both design crafts

The system SHALL expose named roles for the design specializations the catalogue
posts, so a title does not collapse into the bare category role: on the product
side `visual_designer`, `brand_designer`, `motion_designer`, `web_designer`,
`ux_researcher`, `art_director`, `creative_director`, `design_ops`,
`industrial_designer`, and `design_engineer`; on the engineering side
`mechanical_designer`, `electrical_designer`, `civil_designer`, `pcb_designer`,
and `chip_designer` (the last two sitting inside the `hardware` category, whose bare
role would otherwise be all a silicon title gets). The `engineering_design` category SHALL also carry a role
noun, so it yields a bare role and its seniority composites like every other
decomposable category. Longest-alias-first resolution MUST make a qualified
engineering title match its specific role rather than a shorter design alias
contained inside it.

#### Scenario: A qualified engineering title takes the specific role

- **WHEN** the roles of "Senior Mechanical Design Engineer" are derived
- **THEN** they include `mechanical_designer` and not `design_engineer`

#### Scenario: A design specialization is pickable

- **WHEN** the roles of "Senior Visual Designer" are derived
- **THEN** they include `visual_designer` and its graded composite `senior_visual_designer`

#### Scenario: The new category yields a bare role

- **WHEN** the roles of a job with category `engineering_design` and no seniority are derived
- **THEN** they include the bare `engineering_design` role, whose catalog label is a human noun

### Requirement: The skill dictionary covers the design and CAD toolchains

The system SHALL resolve the design craft's tools and practices from a job
description — the Adobe suite beyond Photoshop, the interface-design and
prototyping tools, and the named practices (prototyping, wireframing, design
systems, user research, usability testing, interaction design, design thinking,
typography, accessibility) — and the CAD/EDA stack the engineering side states
(SolidWorks, CATIA, Creo, SketchUp, Altium, ANSYS, and their peers).

Because this dictionary runs over the description of EVERY posting, an alias whose
lowercase form is ordinary English, a person's name, an unrelated product, or a
manual trade MUST NOT resolve on its own. Two remedies apply, and the choice
depends on whether a real design posting would name a strong token beside it:

- `sketch`, `maya`, `blender` and `accessibility` resolve ONLY when corroborated by
  an unambiguous technical token in the same text;
- `principle`, `eagle`, a bare `nx`, `framer` and `spline` resolve not at all —
  their other sense dominates even in corroborated text (`framer` is both a
  carpentry trade and a React animation library; `spline` is the splined shaft of
  the mechanical population this very split describes). A product excluded this way
  MAY still be reachable through an unambiguous phrase (`ptc creo`, `siemens nx`).

#### Scenario: A design tool stated in the description is tagged

- **WHEN** a description states "you will work in Figma and Adobe Illustrator, building prototypes"
- **THEN** the derived skills include `illustrator` and `prototyping`

#### Scenario: A CAD tool stated in the description is tagged

- **WHEN** a description states "3D modelling in SolidWorks and PTC Creo"
- **THEN** the derived skills include `solidworks` and `creo`

#### Scenario: A gated word needs corroboration

- **WHEN** a description states "sketch out ideas each morning" with no technical token
- **THEN** no skill is derived from it; the same word beside "Figma" does resolve to `sketch`

#### Scenario: An excluded homonym never resolves

- **WHEN** a description states "Framer needed for residential construction" or
  "design splined shafts for gearboxes"
- **THEN** no skill is derived from those words, in corroborated text or otherwise
