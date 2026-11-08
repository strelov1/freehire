# telegram-ingest Specification

## Purpose

TBD - created by archiving change add-telegram-source. Update Purpose after archive.

## Requirements

### Requirement: Channels to crawl are configured in a file

The system SHALL read the set of Telegram channels to crawl from a
`sources/telegram.yml` file at crawl startup, each entry naming a `channel` (public
username) and a `kind` (`authored` or `board`). An entry with an unknown
`kind`, an empty `channel`, or a duplicate `channel` SHALL cause the crawl
command to fail fast at startup rather than silently skip or double-crawl the
channel.

#### Scenario: Configured channels are loaded

- **WHEN** `sources/telegram.yml` lists a channel with a valid `kind`
- **THEN** the crawl run includes that channel

#### Scenario: Invalid entry fails fast

- **WHEN** `sources/telegram.yml` contains an entry with an unknown `kind` or a
  duplicate `channel`
- **THEN** the crawl command exits with an error naming the offending entry and
  crawls nothing

### Requirement: Posts are fetched from the public web preview

The crawl SHALL fetch each configured channel's latest posts from the public
web preview (`t.me/s/<channel>`) without a Telegram account, parsing each
post's message id, timestamp, and text. A channel whose fetch fails or whose
page yields no parseable posts SHALL be recorded as failed without aborting
the rest of the run.

#### Scenario: Posts are parsed from the preview page

- **WHEN** a configured channel's preview page contains posts
- **THEN** the crawl yields each post with its message id, timestamp, and text

#### Scenario: A failing channel does not abort the run

- **WHEN** one channel's fetch errors or parses to zero posts and the others
  succeed
- **THEN** that channel is counted as failed and every other channel is still
  crawled

### Requirement: Posts are stored idempotently keyed by channel and message id

The crawl SHALL store each fetched post in a durable `telegram_posts` record
keyed by `(channel, msg_id)`, inserting new posts and leaving already-stored
posts untouched, so that re-crawling a channel never re-processes a seen post.
Stored posts SHALL persist after extraction completes.

#### Scenario: Re-crawl does not duplicate or reset a post

- **WHEN** a post already stored (extracted or pending) appears again in a
  crawl
- **THEN** the stored record is unchanged

### Requirement: Obvious non-vacancy posts are filtered before extraction

The crawl SHALL apply a heuristic prefilter at insert: a post with no vacancy
markers SHALL be stored as already-processed with zero vacancies, so it is
never sent to the LLM, while remaining recorded against re-crawls. The filter
SHALL favor recall: any post with plausible vacancy markers proceeds to
extraction.

#### Scenario: A non-vacancy post is recorded but not queued

- **WHEN** a crawled post contains no vacancy markers
- **THEN** it is stored as processed with zero vacancies and is not claimable
  by the extraction worker

### Requirement: Vacancies are extracted from posts through a durable queue

The system SHALL extract vacancies from pending posts via an LLM behind the
existing provider abstraction. The extraction worker SHALL claim pending posts
with a lease such that concurrent workers never process the same post, SHALL
treat an expired lease as reclaimable, and SHALL extract zero or more
structured vacancies per post (title, company, location, remote hint,
description — salary and contacts stay inside the description, which the
existing enrichment later structures). An extraction payload that fails validation SHALL be
retried once and then dead-lettered; an invalid payload SHALL never be
persisted. Zero extracted vacancies SHALL be a normal success.

#### Scenario: A post yields multiple vacancies

- **WHEN** a pending post describes two distinct roles
- **THEN** extraction yields two structured vacancies from that one post

#### Scenario: Invalid payload is dead-lettered, not persisted

- **WHEN** the LLM returns a payload that fails validation twice for the same
  post
- **THEN** the post is marked dead-lettered with the error recorded and no job
  is written

#### Scenario: A non-vacancy post completes with zero jobs

- **WHEN** extraction determines a claimed post contains no vacancy
- **THEN** the post is marked processed with zero jobs written

### Requirement: Extracted vacancies enter the catalogue through the canonical job write path

Extracted vacancies SHALL be written through the existing job upsert in the
same transaction as marking the post processed. Each job SHALL carry
`source = "telegram"`, `external_id = "<channel>/<msg_id>/<n>"` (stable per
post), the post's URL (`https://t.me/<channel>/<msg_id>`) as the job URL, the
post timestamp as `posted_at`, and the company upserted from the extracted
company name via the existing slug normalization. Writing through the
canonical upsert SHALL enqueue enrichment exactly as for ATS-ingested jobs.

#### Scenario: Extracted job is stored with telegram identity

- **WHEN** extraction yields one vacancy from post 392 in channel `hrlunapark`
- **THEN** the stored job has `source = "telegram"`,
  `external_id = "hrlunapark/392/0"`, and url `https://t.me/hrlunapark/392`

#### Scenario: Extracted jobs flow into enrichment

- **WHEN** extraction writes a new job
- **THEN** an enrichment outbox entry is enqueued for it in the same write

### Requirement: Standalone commands run crawl and extraction on a schedule

The system SHALL provide a standalone crawl command and a standalone extraction
command, each loading configuration, processing one bounded batch (every
configured channel once; a bounded batch of pending posts), reporting counts,
and exiting — suitable for scheduled execution. The crawl command SHALL require
only the database; the extraction command SHALL additionally require the LLM
configuration.

#### Scenario: Crawl command runs once and exits

- **WHEN** the crawl command is run
- **THEN** it crawls every configured channel once and exits after reporting
  stored and failed counts

#### Scenario: Extraction command drains a batch and exits

- **WHEN** the extraction command is run with pending posts
- **THEN** it claims and processes a bounded batch, reports processed, job, and
  failure counts, and exits
