## ADDED Requirements

### Requirement: Coverage is measured off the request path

The system SHALL measure semantic-embedding coverage in `cmd/rollup-stats` and publish
it as a snapshot in the cache. A posting is **eligible** when it is open
(`closed_at IS NULL`), canonical (`duplicate_of IS NULL`), technical
(`is_tech IS TRUE`) and public (`NOT is_private`) — the gate `EnqueuePendingSemanticJobs`
applies, less the private postings it also embeds, which no public figure may count. An eligible
posting is **covered** when `semantic_embedded_model` equals the current embedder model
and `semantic_embedded_hash` is not distinct from its `content_hash`. No HTTP request
SHALL count eligible or covered postings.

#### Scenario: The rollup publishes the snapshot

- **WHEN** `cmd/rollup-stats` runs and the coverage query succeeds
- **THEN** a snapshot holding the eligible count, the covered count and the time of
  the measurement is stored in the cache

#### Scenario: A failed measurement does not fail the run

- **WHEN** the coverage query fails or times out
- **THEN** the run logs it, keeps the previously published snapshot, and exits with the
  same code it would have without this pass

### Requirement: The queue depth is sampled live

The API server SHALL sample the number of claimable `semantic_outbox` rows
(`failed_at IS NULL`) every 15 seconds and keep the samples from the last 10 minutes in
memory. The **drain rate** SHALL be the sum of the depth's decreases between
consecutive samples divided by the time those samples span, so a batch of postings
enqueued at once does not read as negative progress. A server that has held samples
for less than 5 minutes SHALL report no rate.

#### Scenario: Rate from a draining queue

- **WHEN** the samples over 10 minutes go 1000, 900, 800, … , 0 evenly
- **THEN** the drain rate is 100 postings per minute

#### Scenario: An enqueue does not cancel the rate

- **WHEN** the depth falls by 300, jumps up by 5000 as the hourly enqueue lands, then
  falls by 300 again, over 6 minutes
- **THEN** the drain rate is 100 postings per minute, not negative

#### Scenario: A freshly started server

- **WHEN** the server has sampled for 2 minutes
- **THEN** the progress reports the queue depth and no rate or ETA

### Requirement: Progress is served as JSON and as a stream

`GET /api/v1/stats/embeddings` SHALL return `{"data": progress}` where progress
carries: `eligible`, `covered`, `snapshot_covered`, `queued`, `rate_per_minute`,
`eta_seconds`, `snapshot_at` and `measured_at`. `snapshot_covered` is the covered
count the snapshot itself measured. `covered` SHALL be `eligible − queued` clamped to
`[0, eligible]`. `eta_seconds` SHALL be `queued / rate` and absent when the rate is
unknown or zero. Any figure not measured SHALL be `null`, never `0`.
`GET /api/v1/stats/embeddings/stream` SHALL send the same progress object as a
server-sent `progress` event on connect and after every sample, with a keepalive ping
between, and SHALL limit connections per IP.

#### Scenario: No snapshot yet

- **WHEN** no coverage snapshot has been published
- **THEN** `eligible`, `covered`, `snapshot_covered` and `snapshot_at` are `null` and `queued` is still
  reported

#### Scenario: The stream follows the samples

- **WHEN** a client is connected to the stream and the sampler takes a sample
- **THEN** the client receives one `progress` event carrying that sample's depth

#### Scenario: A client that reconnects too often

- **WHEN** one IP opens more stream connections per minute than the limit
- **THEN** the excess connections are refused with HTTP 429

### Requirement: The public progress page

The site SHALL serve a public `/embeddings` page rendered on the server from
`GET /api/v1/stats/embeddings` and then updated from the stream without a reload. It
SHALL show a progress bar, covered of eligible, the snapshot's own covered count with
its time, the queue, the drain rate, the ETA and when the figures were measured, and SHALL invite the reader to speed it up by running
the volunteer client, linking its repository and its run guide. A figure that is `null`
SHALL render as "measuring…" and never as `0`.

#### Scenario: First paint

- **WHEN** a visitor opens `/embeddings`
- **THEN** the HTML already contains the current figures, before any script runs

#### Scenario: Live update

- **WHEN** the page is open and a `progress` event arrives
- **THEN** the figures on the page change to the event's without a reload

#### Scenario: Unmeasured figures

- **WHEN** the progress carries `null` for the rate
- **THEN** the page shows "measuring…" where the rate would be
