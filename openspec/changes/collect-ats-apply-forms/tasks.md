## 1. Schema

- [x] 1.1 Verify the next free migration number against production before writing the file (numbers have collided here from unmerged branches); write `migrations/00NN_apply_forms.sql` creating `apply_forms` (one current form per job, JSONB payload, provider and captured-at provenance) and `apply_form_outbox` (columns and indexes mirroring `semantic_outbox`: `job_id`, `attempts`, `claimed_at`, `failed_at`, `last_error`, `created_at`, unique on `job_id`)
- [x] 1.2 Add the queries to `internal/db/queries/` — upsert a job's form, enqueue a capture gated on the job having none, claim a leased batch (`SKIP LOCKED`, freshest job first), complete, fail with bounded attempts — then `make sqlc`

## 2. The captured form's shape

- [x] 2.1 Create `internal/applyform` with the typed envelope for a captured form: the provider, the capture time, and the field list carrying each control's platform identifier, type, required flag, question text and enumerated options with their platform values
- [x] 2.2 Write the Recruitee mapper: `open_questions` (with `open_question_options`) and the `options_cv|options_phone|options_cover_letter|options_photo|options_salutation` standard-field flags mapped to required/optional/absent. `dynamic_fields` was dropped from this task: it is present but empty on every one of ~150 live offers sampled across 25 boards, so its populated shape has never been observed and a mapper for it would be a guess
- [x] 2.3 Write the Greenhouse mapper over `questions[].fields[]`, keeping `name` as the identifier and the numeric option `value`s intact, and covering `compliance` and `location_questions`
- [x] 2.4 Write the Ashby mapper over `sections[].fieldEntries[]`, reading `path`, `type`, `isRequired`, `title` and `selectableValues` out of the entry's `field` JSON

## 3. Recruitee: the free path

- [x] 3.1 Widen `sources.Job` with an optional application form, nil by default, and assert that an adapter yielding none behaves exactly as today
- [x] 3.2 Have the Recruitee adapter parse and attach the form it already receives, issuing no additional request
- [x] 3.3 Persist an attached form in the ingest write path, in the same transaction as the job's upsert

## 4. Greenhouse and Ashby: the queued path

- [x] 4.1 Enqueue a capture from the ingest write path for a job of a provider whose form needs its own request, only when that job has no current form, and never for a provider with no readable form
- [x] 4.2 Make a failed enqueue unable to fail the crawl, and cover that a re-ingested posting that already has a form queues nothing
- [x] 4.3 Write the Greenhouse fetcher — `boards-api` per-posting call with `questions=true`, against the single host the existing adapter already uses (it serves EU-hosted boards too, so there is no EU base URL to honour, unlike Lever) — and the Ashby fetcher, whose GraphQL selection must take `field` as a scalar (a selection set on it fails the whole query)

## 5. The worker

- [x] 5.1 Write the drain runner in `internal/applyform`: claim a batch, fetch each job's form through its provider's fetcher, store it, bound concurrency, and keep one failure from touching any other capture
- [x] 5.2 Record a failure with its message, retry it on a later run, and stop claiming it once attempts are exhausted
- [x] 5.3 Add `cmd/capture-apply-form` over `worker.Main`/`worker.Bootstrap`, run-once-and-exit, exiting non-zero when the run had failures or dead-letters

## 6. Finish

- [x] 6.1 Run `go build ./...`, `go vet ./...` and `go vet -tags=integration ./...`, then both test passes (`go test ./...` and the tagged suite for the packages touched)
- [x] 6.2 Update `internal/pipeline/AGENTS.md` and `internal/sources/AGENTS.md` for the adapter's new optional yield and the post-ingest capture queue, and add the worker to the `cmd/` notes in `CLAUDE.md`
