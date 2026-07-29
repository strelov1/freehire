## MODIFIED Requirements

### Requirement: Ingest persists job geography and work mode

The ingest write path SHALL parse each posting's `location` string into
`countries`/`regions`/`work_mode` (via the job-geography parser) and persist them
on the job row. These columns SHALL be written on insert and refreshed on
re-ingest, like the other raw source fields and unlike the enrichment payload
(which ingest never writes). A posting whose location yields no geography SHALL
store empty arrays.

For `work_mode`, when the adapter exposes a STRUCTURED work mode (a workplace-type
enum or an explicit remote flag from the ATS API) it SHALL take precedence over
the parser's free-text heuristic; the parser fills `work_mode` only when the
adapter has no structured signal.

When an ATS exposes BOTH a work-mode field and a boolean flag for the same posting, the
adapter SHALL resolve the mode from the field, and MAY fall back to the flag only for
postings that omit the field — for a posting that carries the field, a flag that merely
separates onsite from every other arrangement SHALL NOT be mapped to `remote`, because
such a flag is set on hybrid postings too. Where an ATS splits the signal across separate
`remote` and `hybrid` booleans, both false SHALL yield no work mode rather than a guessed
`onsite`, since the API cannot distinguish "marked as office" from "not marked at all";
both true SHALL yield `remote`, the broader arrangement.

#### Scenario: A new posting stores its parsed geography

- **WHEN** a posting with location `Remote - Germany` is ingested
- **THEN** the stored job has `countries=[de]` and `regions` including `eu`

#### Scenario: Re-ingest refreshes geography from the updated location

- **WHEN** an already-ingested posting is re-ingested with its location changed
  from `Remote - UK` to `Remote - USA`
- **THEN** the job's `countries` updates to `[us]` and its `regions` update
  accordingly

#### Scenario: A location with no geography stores empty arrays

- **WHEN** a posting with location `Remote` is ingested
- **THEN** the stored job has empty `countries` and empty `regions`

#### Scenario: A structured adapter work mode is persisted over the parsed one

- **WHEN** an adapter reports a structured `work_mode` (e.g. Lever's
  `workplaceType=hybrid`) for a posting whose location parses as `remote`
- **THEN** the stored `jobs.work_mode` is the structured value `hybrid`

#### Scenario: An Ashby hybrid posting is not remote

- **WHEN** an Ashby posting carries `workplaceType=Hybrid` together with `isRemote=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: An Ashby posting without a workplace type falls back to the flag

- **WHEN** an Ashby posting omits `workplaceType` and carries `isRemote=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A Recruitee hybrid offer keeps its work mode

- **WHEN** a Recruitee offer carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: A SmartRecruiters hybrid posting keeps its work mode

- **WHEN** a SmartRecruiters posting's location carries `remote=false` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `hybrid` and the job is not marked remote

#### Scenario: Neither remote nor hybrid leaves the work mode unknown

- **WHEN** an offer carries `remote=false` and `hybrid=false`
- **THEN** the yielded job carries no structured `work_mode`, leaving the decision to the
  pipeline's location parser

#### Scenario: An offer flagged both remote and hybrid resolves to remote

- **WHEN** an offer carries `remote=true` and `hybrid=true`
- **THEN** the yielded job's `work_mode` is `remote`

#### Scenario: A BambooHR posting takes its work mode from locationType

- **WHEN** a BambooHR careers-list posting carries `locationType` `0`, `1`, or `2` with a
  null `isRemote`
- **THEN** the yielded job's `work_mode` is `onsite`, `remote`, or `hybrid` respectively,
  and only the `remote` one is marked remote
