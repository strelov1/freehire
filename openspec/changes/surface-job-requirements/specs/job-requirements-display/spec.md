## ADDED Requirements

### Requirement: The job detail page renders the posting's stated requirements

The job detail page SHALL render the posting's stated requirements
(`enrichment.requirements`) as a section of the page's main reading column,
positioned after the posting's description and before the skills section. The
section SHALL be titled in the same voice as the sections around it and SHALL
present each requirement's `text` verbatim as stored, without truncation,
re-wording, or de-duplication against any other facet already shown on the page.

Requirements SHALL be grouped by `priority` into a `required` group followed by
a `preferred` group, in that fixed order. A group with no entries SHALL render
no heading of its own.

The requirement text is lifted from the posting rather than synthesized, so the
section SHALL be rendered in the posting's own content language, the same way
the description body is — unlike the model-written summary, which is pinned to
English.

#### Scenario: A job with both priorities renders two groups in order

- **WHEN** the job detail page renders a job whose `enrichment.requirements` holds entries of both `required` and `preferred` priority
- **THEN** the page shows a requirements section containing a `required` group listing every `required` entry, followed by a `preferred` group listing every `preferred` entry

#### Scenario: A single-priority list renders only that group

- **WHEN** the job detail page renders a job whose `enrichment.requirements` holds only `required` entries
- **THEN** the requirements section lists those entries and shows no `preferred` group heading

#### Scenario: A job with no requirements renders no section

- **WHEN** the job detail page renders a job whose `enrichment.requirements` is empty or absent
- **THEN** the page shows no requirements section at all — no heading, no empty state, no placeholder

#### Scenario: Requirement text is shown verbatim

- **WHEN** a requirement's `text` restates something already displayed elsewhere on the page, such as a skill that also appears as a skill chip
- **THEN** the requirement is still listed in the requirements section, unchanged

#### Scenario: The section takes the posting's language

- **WHEN** the job detail page renders a job whose posting language is not English
- **THEN** the requirements section is marked with the posting's content language, matching the description body rather than the summary
