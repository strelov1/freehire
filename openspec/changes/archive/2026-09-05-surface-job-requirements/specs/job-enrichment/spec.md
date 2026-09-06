## ADDED Requirements

### Requirement: The derived requirements fill the served field when the model states none

The served `enrichment.requirements` SHALL be filled from a job's stored,
deterministically derived requirements whenever the enrichment payload being
written states none of its own. The model's reading wins when it has one: it
reads the postings whose requirements are prose with no list markup, which the
derivation cannot reach, so the two sources add coverage rather than compete.

The fold SHALL happen on the READ path, where the projection assembles the served
payload, and the derived list SHALL NOT be copied into the stored enrichment blob.
Two attempts to do it at write time both failed, in ways worth stating as
requirements of their own:

- A write-time overlay runs only when the model runs, so it fills nothing for the
  postings the model has never reached — which are the majority, and the whole
  reason the derivation exists.
- A copy in the blob is a second stored value that nothing revises. A later crawl
  rewrites the column and leaves the copy, so a description edit that removed the
  requirements section would leave a consumer reading a list the posting no longer
  states, out of reach of any backfill.

Reading the column on every projection satisfies both: the served field always
reflects the current column, and a consumer still reads exactly one field.

A later enrichment run therefore SHALL NOT be able to erase a derived list — the
run writes only what the model said, and the projection re-reads the column.

The derivation SHALL NOT change a job's enrichment version or provenance stamp:
it is orthogonal to the model payload, and a job that has never been enriched
stays that way while still serving derived requirements.

#### Scenario: A model payload with requirements is left alone

- **WHEN** a job whose stored enrichment states a non-empty `requirements` list also has stored derived requirements
- **THEN** the job's served `enrichment.requirements` is the payload's list, unchanged

#### Scenario: A model payload without requirements picks up the derivation

- **WHEN** a job whose stored enrichment states no `requirements` has non-empty stored derived requirements
- **THEN** the job's served `enrichment.requirements` is the derived list

#### Scenario: Neither source yields a list

- **WHEN** a job whose stored enrichment states no `requirements` also has empty stored derived requirements
- **THEN** the job's served `enrichment.requirements` is empty or absent

#### Scenario: An unenriched job still serves derived requirements

- **WHEN** a client requests a job that has never been enriched but whose stored derived requirements are non-empty
- **THEN** the returned object's `enrichment.requirements` holds the derived list and the job's enrichment provenance still reports it as unenriched

#### Scenario: A later enrichment run cannot erase a derived list

- **WHEN** a job serving derived requirements is enriched by a run whose payload states no requirements of its own
- **THEN** the job still serves the derived list afterwards

#### Scenario: The derived list is not copied into the stored enrichment

- **WHEN** an enrichment payload stating no `requirements` is written for a job whose stored derived requirements are non-empty
- **THEN** the job's stored enrichment still states no `requirements`, and the derived column is unchanged
