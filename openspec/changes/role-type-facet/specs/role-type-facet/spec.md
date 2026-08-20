## ADDED Requirements

### Requirement: A people-management role type is resolved from the title

The system SHALL provide `internal/roletype`, a curated dictionary that resolves a
job title to the canonical value `people_manager` or to the empty string. It SHALL
follow the doctrine of `internal/classify`, `internal/roletag` and
`internal/skilltag`: whole-word matching, no inference, and nothing emitted for what
it cannot resolve.

`vocab.RoleTypeValues` SHALL hold the vocabulary. It contains exactly one value,
`people_manager`. The absence of a value SHALL mean "no management marker in the
title", never "individual contributor" — a posting whose title states nothing about
management is unresolved, not resolved to the opposite.

The dictionary SHALL recognise the unambiguous markers `head of`, `director`, `vp`,
`vice president`, `chief`, and `supervisor`, plus a manager qualified by a craft
(`engineering manager`, `data manager`, `qa manager`, and like forms).

#### Scenario: An unambiguous marker resolves

- **WHEN** `Derive` is given "Director of Data Engineering"
- **THEN** it returns `people_manager`

#### Scenario: A craft-qualified manager resolves

- **WHEN** `Derive` is given "Engineering Manager, Payments"
- **THEN** it returns `people_manager`

#### Scenario: A title stating nothing about management resolves to nothing

- **WHEN** `Derive` is given "Backend Engineer"
- **THEN** it returns the empty string, not a value meaning individual contributor

#### Scenario: An empty title resolves to nothing

- **WHEN** `Derive` is given an empty or whitespace-only title
- **THEN** it returns the empty string

### Requirement: Manager-titled individual-contributor roles are masked out

Many titles contain the word "manager" while naming no people-management role:
a Product Manager manages a product, an Account Manager an account, a Project or
Program Manager a plan. On production these are 150,779 of the 378,410 titles
containing "manager" — 40% — so admitting the bare word would make the facet wrong
for two in five of its own matches.

The dictionary SHALL therefore NOT treat a bare "manager" as a marker. It SHALL
resolve only the craft-qualified forms, and it SHALL carry an explicit blind-phrase
list for the non-management "… manager" roles so that a phrase reaching the matcher
by another route still cannot resolve. The list mirrors the vocabulary
`internal/classify` already curates for the same titles.

#### Scenario: Product Manager is not a people manager

- **WHEN** `Derive` is given "Senior Product Manager"
- **THEN** it returns the empty string

#### Scenario: Project, Program and Account Manager are not people managers

- **WHEN** `Derive` is given "Project Manager", "Program Manager" or
  "Account Manager"
- **THEN** it returns the empty string for each

#### Scenario: A bare Manager does not resolve

- **WHEN** `Derive` is given "Manager"
- **THEN** it returns the empty string, because the title names no craft and the
  word alone does not distinguish the two senses

#### Scenario: A masked phrase does not shadow a real marker in the same title

- **WHEN** `Derive` is given "Director of Product Management"
- **THEN** it returns `people_manager`, because `director` is an unambiguous marker
  regardless of the product-management phrase beside it

### Requirement: Lead is deliberately unresolved

"Lead" SHALL NOT be a management marker. In this catalogue `seniority=lead` holds
116,893 postings of which only 3,303 carry any management marker — the word
overwhelmingly names the individual-contributor ladder (Tech Lead, Lead Engineer).
Resolving it either way would be a guess, so the dictionary emits nothing and the
posting stays unresolved.

#### Scenario: Tech Lead is unresolved

- **WHEN** `Derive` is given "Tech Lead" or "Lead Software Engineer"
- **THEN** it returns the empty string

### Requirement: The role type is derived at index time, not stored

`role_type` SHALL be computed in `search.FromJob` from the job's title, alongside
`roles` and `ai_archetype`, and carried as a top-level field on the search
document. There SHALL be no `jobs` column, no migration, and no
`cmd/backfill-derive` participation: a reindex is what reaches existing postings,
and incremental indexing reaches new ones.

#### Scenario: A document carries the derived role type

- **WHEN** a job titled "Head of Platform Engineering" is turned into a search
  document
- **THEN** the document's top-level `role_type` is `people_manager`

#### Scenario: An unresolved job carries no role type

- **WHEN** a job titled "Backend Engineer" is turned into a search document
- **THEN** the document's `role_type` is absent or empty, and the job is still
  indexed and findable
