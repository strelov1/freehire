## MODIFIED Requirements

### Requirement: Repeated failures are retried then dead-lettered

An extraction that fails validation SHALL be retried at most once within the same
attempt before the attempt is counted as failed. On a failed attempt the system SHALL
increment the entry's `attempts` and record the error, leaving its lease in place so
the entry is retried on a later run after the lease expires (never reprocessed within
the same run); no invalid payload SHALL ever be written to `jobs`.

Whether a failed entry is dead-lettered SHALL depend on whether the posting caused
the failure:

- A failure **the posting caused** — a corrupted row, a model response that cannot be
  parsed for this input, or a payload that fails validation — SHALL be dead-lettered
  once `attempts` reaches the configured maximum.
- **Every other failure**, including any transport, gateway, authentication or timeout
  error, SHALL NOT be dead-lettered on the attempt counter. It SHALL be dead-lettered
  only once the entry has been queued longer than a configured grace window.

The classification SHALL be defined as the set of failures the system itself raises
for a posting, so an unrecognised failure counts as not the posting's fault. An
outage shorter than the grace window SHALL therefore cost no entry permanently,
however many attempts it consumes.

#### Scenario: A transient failure is retried on a later run

- **WHEN** enriching an entry fails once (validation or LLM error) and it is not yet
  eligible for dead-lettering
- **THEN** the job is left unenriched, the entry's `attempts` is incremented, and the
  entry becomes eligible to be claimed again only after its lease expires

#### Scenario: A posting that cannot be enriched is dead-lettered

- **WHEN** an entry fails with an unparseable model response until its attempts reach
  the configured maximum
- **THEN** its `failed_at` is set, it is no longer claimed, and the job's `enrichment`
  was never written with an invalid value

#### Scenario: A gateway outage does not dead-letter an entry

- **WHEN** an entry fails repeatedly with a gateway error, past the attempt maximum,
  while it has been queued for less than the grace window
- **THEN** its `failed_at` remains unset and it stays claimable, so the entry survives
  the outage and is enriched once the gateway recovers

#### Scenario: An entry failing on our side indefinitely still stops

- **WHEN** an entry has failed with a non-posting error and has been queued longer
  than the grace window
- **THEN** its `failed_at` is set, so a permanently unservable entry cannot consume
  the queue forever

#### Scenario: An unrecognised failure is not blamed on the posting

- **WHEN** an entry fails with an error the classifier does not recognise
- **THEN** it is treated as not the posting's fault and bounded by the grace window
  rather than by the attempt counter
