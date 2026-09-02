## MODIFIED Requirements

### Requirement: Catalogue-fit rule decides which jobs are removed

The system SHALL decide a job's catalogue fit from three independent rules, evaluated live against the current non-tech dictionary rather than from the stored `is_tech` column, so an iteration needs no prior re-derivation pass. A job MUST be removed when the non-tech title detector flags its title, when its category is one of the non-technical categories AND its company has never shown any technical evidence, or when its `is_tech` is unknown AND its company has never shown any technical evidence nor any tagged skill. A job from a source that is never re-crawled — Telegram extraction, user submissions, link-source imports — MUST NOT be removed by any rule, because a dictionary mistake there cannot be undone by a later crawl.

The category rule MUST subtract the non-technical categories that name a CRAFT rather than back-office or go-to-market work. Those categories are non-technical because the discipline sits outside IT — an IT job board is not where a draughtsman or a process engineer looks for work — not because the posting is a business role at a software employer, and deleting them would take out an engineering employer's whole catalogue the moment its board is retired. That subtraction MUST be expressed as a named vocabulary set, not as categories named inline at the rule, and the set MUST be a subset of the non-technical categories, enforced by a test: a category named inline cannot express a set, and the next craft category added would silently become deletable.

A company's technical evidence MUST be evaluated over its entire history including closed jobs, because "this company never posts anything technical" needs the maximum available evidence, and MUST be computed once per run before any deletion so the classification cannot shift underneath the run.

#### Scenario: Blue-collar title is removed everywhere

- **WHEN** a job's title is flagged by the non-tech title detector
- **THEN** the job is removed regardless of its company's technical evidence

#### Scenario: Business role at a non-technical company is removed

- **WHEN** a job's category is a non-technical category and its company has never posted a job with technical evidence
- **THEN** the job is removed

#### Scenario: Business role at a technical company is kept

- **WHEN** a job's category is a non-technical category but its company has posted jobs with technical evidence
- **THEN** the job is kept

#### Scenario: A non-technical craft category is never business work

- **WHEN** a job's category is `engineering_design` or `industrial_engineering` and its company has never posted a job with technical evidence
- **THEN** the job is kept: the category rule subtracts the craft set before it applies

#### Scenario: The craft set cannot drift from the vocabulary

- **WHEN** the vocabulary is checked
- **THEN** every member of the craft set is also a member of the non-technical categories

#### Scenario: Unknown job at a company with no evidence at all is removed

- **WHEN** a job's `is_tech` is unknown and its company has never shown technical evidence nor any tagged skill across its whole history
- **THEN** the job is removed

#### Scenario: Unknown job at a company with some evidence is kept

- **WHEN** a job's `is_tech` is unknown and its company has posted at least one job with technical evidence or tagged skills
- **THEN** the job is kept

#### Scenario: A non-crawled source is never removed

- **WHEN** a job originates from Telegram extraction, a user submission, or a link-source import and would otherwise match a removal rule
- **THEN** the job is kept
