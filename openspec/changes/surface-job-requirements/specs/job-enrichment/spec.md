## ADDED Requirements

### Requirement: The derived requirements fill the served field when the model states none

The served `enrichment.requirements` SHALL be filled from a job's stored,
deterministically derived requirements whenever the enrichment payload being
written states none of its own. The model's reading wins when it has one: it
reads the postings whose requirements are prose with no list markup, which the
derivation cannot reach, so the two sources add coverage rather than compete.

This overlay SHALL be applied by the same write that stores an enrichment
payload, chained after the salary overlays that write already carries, so a
consumer continues to read exactly one field. It SHALL apply to the moderator
re-create write path as well, so a re-created posting does not lose its derived
list.

Because the overlay is applied at write time, a later enrichment run SHALL NOT
be able to erase a derived list: the run either replaces it with the model's own
reading or the overlay restores it.

The derivation SHALL NOT change a job's enrichment version or provenance stamp:
it is orthogonal to the model payload, and a job that has never been enriched
stays that way while still serving derived requirements.

#### Scenario: A model payload with requirements is left alone

- **WHEN** an enrichment payload stating a non-empty `requirements` list is written for a job that also has stored derived requirements
- **THEN** the job's served `enrichment.requirements` is the payload's list, unchanged

#### Scenario: A model payload without requirements picks up the derivation

- **WHEN** an enrichment payload stating no `requirements` is written for a job whose stored derived requirements are non-empty
- **THEN** the job's served `enrichment.requirements` is the derived list

#### Scenario: Neither source yields a list

- **WHEN** an enrichment payload stating no `requirements` is written for a job whose stored derived requirements are empty
- **THEN** the job's served `enrichment.requirements` is empty or absent

#### Scenario: An unenriched job still serves derived requirements

- **WHEN** a client requests a job that has never been enriched but whose stored derived requirements are non-empty
- **THEN** the returned object's `enrichment.requirements` holds the derived list and the job's enrichment provenance still reports it as unenriched

#### Scenario: A later enrichment run cannot erase a derived list

- **WHEN** a job serving derived requirements is enriched by a run whose payload states no requirements of its own
- **THEN** the job still serves the derived list afterwards
