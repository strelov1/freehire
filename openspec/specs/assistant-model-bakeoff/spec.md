# assistant-model-bakeoff

## Purpose

Choosing the model that answers an assistant turn is a measurement, not a reading of a
price list. A per-million price does not predict what a turn costs here: the transcript is
replayed into context every round, so cost grows with ROUNDS as well as tokens, and whether
the provider serves that replayed prefix from its cache decides the rest. Neither figure
appears on a price page.

This capability is the measurement: one tailoring-autopilot run per (candidate model, case)
against a real gateway, scored by the product's own deterministic scores, reported with
every run's tailored CV so a reader judges what no number here can.

## Requirements

### Requirement: Every candidate model runs against the same world

The bake-off SHALL stand a fresh database for each candidate model and seed it from the
same fixtures before that model's runs begin. No run SHALL observe state left by a
previous model's run.

The bake-off SHALL replace only the turn model. The fit-analysis model and every other
model the harness wires SHALL stay fixed across candidates, so a difference in the report
is attributable to the model under test.

#### Scenario: Two models over one case

- **WHEN** the bake-off runs case `C` on model `A` and then on model `B`
- **THEN** model `B` starts from the seeded fixtures and not from the CV `A` produced

#### Scenario: The fit model is held constant

- **WHEN** the bake-off runs the same case on two candidate turn models
- **THEN** both runs resolve the fit analysis through the same fit model

### Requirement: A run reports five cost measurements

For each (model, case) the bake-off SHALL report the number of tool-calling rounds the
turn took before its terminal event, the number of tool calls whose arguments failed to
decode against the tool's schema, the input and output token counts the provider
reported, the count of input tokens the provider served from its prompt cache, and the
elapsed time from the request to the turn's first streamed token.

A count the provider does not report at all SHALL be recorded as absent rather than as
zero, wherever the transport can tell the two apart. It cannot for cached tokens: the LLM
library writes that key unconditionally from a zero value, so an unreported count and a
genuine cache miss both arrive as `0`. The bake-off SHALL therefore report the observed
cached count per run without claiming it distinguishes those cases, and SHALL label a
model whose every round reports zero across a run whose prefix was stable as **no cache
observed** — an inference stated at run scope, never a per-call measurement.

#### Scenario: Cached tokens are reported across a run

- **WHEN** a run completes and its rounds report non-zero cached input counts
- **THEN** the row records the total and the share it represents of the input tokens

#### Scenario: Every round of a run reports zero cached tokens

- **WHEN** a run completes, its history prefix was stable across rounds, and every round
  reported zero cached input tokens
- **THEN** the row is labelled "no cache observed" rather than reporting a cache-hit rate
  of zero as though it were measured

#### Scenario: A malformed tool call

- **WHEN** the model emits a tool call whose arguments do not decode
- **THEN** the row counts it, and the run continues, because a tool failure is not a turn
  failure

### Requirement: Quality is scored deterministically and never by a grader model

The bake-off SHALL score each completed run with `cvmatch.Compute` over the tailored CV
against the case's vacancy, and with `atscheck.Compare` between the base and tailored
reports. Both SHALL be computed without a model call, and the report SHALL rank on them.

The bake-off SHALL NOT call any model to judge a result. Reading whether a tailored CV is
good is the job of whoever reads the report.

#### Scenario: Two models are ranked

- **WHEN** two models' runs over one case are both scored
- **THEN** the report ranks them on the deterministic scores and makes no model call to
  do so

#### Scenario: The scores are close

- **WHEN** two models' deterministic scores on one case are close enough that the ranking
  is not meaningful
- **THEN** the report presents both, and no verdict is invented on the reader's behalf

### Requirement: Every run's tailored CV is in the report

The bake-off SHALL emit each completed run's tailored CV as text in the report, beside
that run's vacancy, its deterministic scores and its cost measurements, so the CVs can be
read and compared without re-running anything.

The report SHALL name the vacancy each CV was tailored against. A CV read without the
posting it was written for cannot be judged.

#### Scenario: A run over several vacancies

- **WHEN** the bake-off runs one model over several cases
- **THEN** the report carries one tailored CV per case, each naming its vacancy

#### Scenario: A run that failed

- **WHEN** a run did not complete
- **THEN** its row carries the failure and no CV text, rather than a partial document
  presented as a result

### Requirement: A failed case does not end the bake-off

The bake-off SHALL record a run that errored, was cancelled, or hit its step ceiling as a
row carrying that outcome and its reason, and SHALL continue with the remaining runs.
Only a failure to read the case set at all SHALL end the bake-off.

#### Scenario: One model fails one case

- **WHEN** a run ends with an error
- **THEN** the row records the failure and its reason, and the remaining models and cases
  still run

#### Scenario: The case set cannot be read

- **WHEN** the case fixtures are missing or unparseable
- **THEN** the bake-off stops and names what it could not read

### Requirement: Prices are read from the catalogue, never hard-coded

The bake-off SHALL compute a run's cost from a checked-in price table generated from the
gateway's own model catalogue, and SHALL record the date that table was captured in the
report. A model absent from the table SHALL be reported with its measurements and without
a cost, never with a cost of zero.

#### Scenario: A candidate model is missing from the price table

- **WHEN** a run completes for a model the price table does not name
- **THEN** the row carries its five measurements and its cost is reported as unknown

#### Scenario: The report is read later

- **WHEN** a report is read
- **THEN** it names the date its prices were captured, so a stale cost is visible as stale
