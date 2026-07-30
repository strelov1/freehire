## ADDED Requirements

### Requirement: Engineering design is a category of its own, distinct from product design

The system SHALL resolve engineering draughting and design work — mechanical,
electrical, civil/structural, and chip design — to a dedicated
`engineering_design` category, and SHALL reserve the `design` category for
product, visual, and experience design. The engineering aliases MUST be ordered
ahead of the bare `design` alias in the title dictionary, so a qualified title
resolves to the engineering category rather than to `design` by virtue of
containing the word "design". `engineering_design` MUST be a member of the
non-technical category set, so it is surfaced as a facet but consumes no LLM
enrichment or embedding budget.

#### Scenario: A qualified engineering-design title leaves the design category

- **WHEN** a job titled "Senior Mechanical Design Engineer" is classified
- **THEN** its category is `engineering_design`, not `design`

#### Scenario: Chip and board design resolve to engineering design

- **WHEN** a job titled "Physical Design Engineer" or "PCB Design Engineer" is classified
- **THEN** its category is `engineering_design`

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

### Requirement: Named roles cover both design crafts

The system SHALL expose named roles for the design specializations the catalogue
posts, so a title does not collapse into the bare category role: on the product
side `visual_designer`, `brand_designer`, `motion_designer`, `web_designer`,
`ux_researcher`, `art_director`, `creative_director`, `design_ops`,
`industrial_designer`, and `design_engineer`; on the engineering side
`mechanical_designer`, `electrical_designer`, `civil_designer`, `pcb_designer`,
and `chip_designer`. The `engineering_design` category SHALL also carry a role
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
(SolidWorks, CATIA, Creo, SketchUp, Altium, ANSYS, and their peers). Aliases whose
lowercase form is ordinary English or an unrelated product — `sketch`,
`principle`, `eagle`, a bare `maya` — MUST NOT be added: in long prose they
resolve falsely, and these dictionaries are precision-first.

#### Scenario: A design tool stated in the description is tagged

- **WHEN** a description states "you will work in Figma and Adobe Illustrator, building prototypes"
- **THEN** the derived skills include `illustrator` and `prototyping`

#### Scenario: A CAD tool stated in the description is tagged

- **WHEN** a description states "3D modelling in SolidWorks and Creo"
- **THEN** the derived skills include `solidworks` and `creo`

#### Scenario: A homonym does not resolve

- **WHEN** a description states "sketch out ideas quickly" or "the guiding principle is simplicity"
- **THEN** no skill is derived from those words
