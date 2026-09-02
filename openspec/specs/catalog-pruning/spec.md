# catalog-pruning Specification

## Purpose
TBD - created by archiving change prune-non-it-catalog. Update Purpose after archive.
## Requirements
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


### Requirement: Company-scoped removal requires retiring the board

The system SHALL apply the company-scoped rules — non-technical category at a company without technical evidence, and unknown at a company with no evidence at all — only to companies whose board entries are removed from the source board files in the same step. Boards are re-crawled hourly on the unchanged dedup key, so a company-scoped deletion that leaves the board in place is undone within one crawl cycle; the ingest-time rejection covers only the title rule and cannot substitute for board retirement.

#### Scenario: Company-scoped deletion without board retirement is refused

- **WHEN** a run would apply a company-scoped rule to a company whose board is still listed in the source board files
- **THEN** the run reports those companies and does not delete their jobs

#### Scenario: Retirement candidates are reported

- **WHEN** the operator requests the board-retirement report
- **THEN** the system lists the board-file entries whose company has never shown technical evidence, identified by the same slug normalization ingest uses

### Requirement: Deletion is batched, capped, and mirrored to the search index

The system SHALL delete matching jobs in bounded batches and MUST extend every batch to the duplicate cluster of each row it deletes, because `jobs.duplicate_of` is a restricting foreign key and a canonical row with a live duplicate cannot be deleted alone. Each batch MUST remove the same rows from the Meilisearch facet index by primary key, so the index does not serve documents whose rows no longer exist. A run MUST honour an operator-supplied cap on the number of rows it deletes.

#### Scenario: Duplicate cluster is deleted with its canonical

- **WHEN** a batch selects a canonical job that other rows reference through `duplicate_of`
- **THEN** the referencing rows are deleted in the same batch and the delete does not fail on the foreign key

#### Scenario: Search index is cleaned in the same step

- **WHEN** a batch of jobs is deleted from the database
- **THEN** the same identifiers are removed from the facet search index

#### Scenario: Run stops at the cap

- **WHEN** a run is given a cap lower than the number of matching rows
- **THEN** the run deletes at most that many rows and reports how many matched

### Requirement: Deletion is dry-run by default and gated on inspection

The system SHALL default to reporting what it would delete and MUST require an explicit flag to delete anything. A dry run MUST print a random sample of the pending batch's titles and MUST break the batch down by rule and by source, so a batch dominated by a single board — the signature of a too-broad term rather than a real cluster — is visible before any row is removed.

#### Scenario: Default run deletes nothing

- **WHEN** the worker runs without the explicit apply flag
- **THEN** it reports the matching rows and deletes none

#### Scenario: Dry run surfaces a sample and a breakdown

- **WHEN** a dry run completes
- **THEN** it prints a random sample of the pending titles and counts grouped by rule and by source

### Requirement: Deletions are archived for audit

The system SHALL record every deleted job in an archive holding its identity, title, company, and the rule that removed it, and MUST NOT copy the description or the enrichment payload — the storage those columns occupy is the reason for the deletion. The archive is the only way to answer, after an irreversible removal, whether something was removed that should have been kept.

#### Scenario: A deleted job leaves an archive row

- **WHEN** a job is deleted by the worker
- **THEN** an archive row records its identifier, source, external id, title, company slug, the rule that matched, and the time

#### Scenario: Archive stays small

- **WHEN** archive rows are written
- **THEN** they carry no description and no enrichment payload

### Requirement: Residual unclassified titles are reportable

The system SHALL provide a read-only report of the most frequent word groups occurring in the titles of open, non-duplicate jobs that still carry no `is_tech` signal, each with the number of distinct jobs containing it and the sources those jobs came from. Clustering MUST be by word group rather than by whole title: boards append location, schedule and requisition detail to titles, so half the unclassified mass is titles that occur exactly once, and whole-title grouping splits one role across many singleton clusters. A word group is also the unit the non-tech dictionary accepts, so a reported cluster is directly usable as an anchored term.

A group MUST be two adjacent words, or three adjacent words whose middle word is a connector. Connectors are function words that belong inside a role name but not at its boundary — in Portuguese and Spanish the preposition is part of the role ("operador de caixa") while the two-word fragments around it ("analista de") are noise — so a connector MUST be permitted only in a three-word group's middle position and MUST NOT edge any group. Groups MUST form only within a run of the title delimited by punctuation, so adjacency never spans a separator and the report cannot invent a phrase that occurs in no title.

Tokenization MUST be Unicode-aware, so accented Latin and non-Latin scripts survive it — a large share of the catalogue is Portuguese, Spanish and Russian. A group MUST NOT contain a stop word anywhere, and its edge words MUST be at least three characters and not purely numeric, since employment type, posting boilerplate, requisition numbers and shredded schedule notation otherwise dominate the ranking. Each job MUST be counted at most once per group, so a group repeated within one title does not inflate its cluster. An absent or empty vocabulary MUST NOT be treated as excluding everything: an empty report is the signal that the residual mass is exhausted, and a missing parameter must never produce it.

#### Scenario: Operator sees the largest remaining clusters

- **WHEN** the report runs with a requested limit
- **THEN** it lists that many word groups by descending count of distinct jobs, each with its count and sources

#### Scenario: A connector is bridged mid-group and rejected at an edge

- **WHEN** a title contains a role name whose middle word is a connector, such as "operador de caixa"
- **THEN** the three-word group is reported and the two-word fragments ending or beginning with that connector are not

#### Scenario: Adjacency does not span punctuation

- **WHEN** two words are adjacent in a title only across a separator, as "aide" and "honolulu" are in "Personal Care Aide - Honolulu"
- **THEN** no group pairs them

#### Scenario: Requisition numbers and shredded notation are not reported

- **WHEN** a title contains a purely numeric token or a token shorter than three characters
- **THEN** no group is edged by it

#### Scenario: A missing vocabulary does not empty the report

- **WHEN** the report runs with no stop words and no connectors supplied
- **THEN** it still reports the clusters present, rather than reporting none

#### Scenario: One role spelled inconsistently forms one cluster

- **WHEN** several jobs share a role phrase but differ in the location, schedule or requisition detail appended to their titles
- **THEN** they are counted in the same word-pair cluster

#### Scenario: Accented and non-Latin titles survive tokenization

- **WHEN** a title contains accented Latin or non-Latin words
- **THEN** those words are tokenized whole rather than split at the non-ASCII character

#### Scenario: Boilerplate is not reported as a cluster

- **WHEN** a title contains employment type, schedule notation or posting boilerplate
- **THEN** no word pair built from those tokens appears in the report

#### Scenario: Classified jobs are excluded

- **WHEN** a job carries a technical or non-technical signal
- **THEN** it does not appear in the residual report

### Requirement: Removal MUST NOT destroy a user's application

A prune run SHALL leave every tracked application standing when it deletes the posting the
application was made against, clearing the application's link to that posting and nothing else.
The run MAY continue to remove the views, saves, dismissals and votes recorded against the
posting: those are marks on inventory, and losing them costs a bookmark.

The distinction is the point. The worker's existing note — that a user's saved job goes with the
posting and this is an accepted cost of the campaign — weighed a bookmark and, through one shared
cascade, silently applied the same verdict to applications. An application carries a date, a
stage, free-text notes and a mail history; it is a record of something a person did, and no
catalogue-hygiene campaign has the standing to delete it.

#### Scenario: A pruned posting leaves the application intact

- **WHEN** a run deletes a posting that a user had applied to
- **THEN** the user's application survives with its date, stage, notes and follow-up mark
- **AND** the application no longer names a posting

#### Scenario: A pruned posting still takes bookmarks with it

- **WHEN** a run deletes a posting that a user had only viewed, saved, dismissed or voted on
- **THEN** those marks are removed with the posting

#### Scenario: Removal does not change what the aggregates say about the employer

- **WHEN** a run deletes a posting that a user had applied to and the employer had replied to
- **THEN** the company's served response rate and median reply time are the same as before the run

#### Scenario: A cap is not a licence

- **WHEN** a run deletes its full capped batch
- **THEN** no application anywhere is deleted as part of that batch

