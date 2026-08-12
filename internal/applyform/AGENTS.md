# Application-form capture conventions

## Scope
Capturing the application form an ATS published for a posting — the questions a candidate
must answer and the identifiers the platform expects back — and projecting it for display
on the job page. Providers: greenhouse, ashby, workable, lever. Queue drain is
`cmd/capture-apply-form`; the read is `GET /jobs/:slug/apply-form`
(`internal/handler/apply_form.go:25`); wire shapes go to TypeScript via `cmd/gen-contracts`.

## Always true
- **Platform vocabulary is stored VERBATIM** — the opposite of `internal/skilltag` and
  `internal/classify` (form.go:1-22). Field IDs, option values and question text are kept
  exactly as the platform sent them: `question_67165648` is a token to hand back to
  Greenhouse, not a name to interpret, and any mapping of it is loss. The ONE normalized
  thing is `FieldType`, the kind of control (form.go:27-54), and it follows the dict-only
  rule: a word the dictionary does not know yields no type, with `RawType` kept alongside
  so nothing is lost.
- **The store exists for a consumer not yet built** — the one that fills a form rather than
  describing it. display.go is the only reader today and wants almost none of it: the
  question text plus one word about the answer. It drops hidden/info/demographic/consent
  controls and folds the standard fields (name, email, CV) into one `basics` line, keyed on
  the identifiers our own mappers produce, not on labels (display.go:51-90, 120-166).
- **One registry map sits behind both gate and drain.** `fetcherFor` (fetch.go:78-83) backs
  both the ingest enqueue gate (`NeedsRequestCapture`, fetch.go:70-73) and the worker's
  fetcher set, so the two cannot drift into a queue full of undrainable work; a test holds
  them to it. Recruitee's form arrives free with the crawl and is written directly at
  ingest (`cmd/ingest/store.go:168` `SaveWithApplyForm`), never queued.
- **404 → `ErrPostingGone` → discard, not retry** (fetch.go:14-38). Across a backlog of a
  quarter-million postings there are thousands the employer took down; retried, each burns
  its attempts and dead-letters, and a queue steadily accumulating dead letters is
  indistinguishable from a broken one. Ashby reports an absent posting as 200 + null, which
  is converted to `ErrPostingGone` too (fetch.go:237-242).
- **Lever needs the posting's own URL.** A posting lives on exactly one of two regional
  hosts and the wrong one answers 404 — the same code as a deleted posting — so the region
  cannot be discovered by trying and is read from the stored URL (fetch.go:142-159;
  runner.go:21-25).
- **`RunStats.Degraded` deliberately rejects the shared `worker.ExitCode` rule**
  (runner.go:90-105). This worker issues hundreds of thousands of requests to other
  companies' APIs, where a handful of transient failures per run is the healthy shape
  (first production run measured 10 in 2490). Only dead letters — work abandoned — or a run
  that captured nothing while failing deserve an alert.
- **`MaxPerRun` exists because nothing holds a flock.** systemd `Type=oneshot` is the only
  anti-stacking mechanism, so an unbounded run over the ~185k backlog works for hours while
  every scheduled firing behind it is silently dropped (runner.go:55-64;
  internal/config/applyform.go:23-28).
- **`Transport` and `statusCoder` are declared here** (fetch.go:27, 46-53) because
  `internal/sources` imports THIS package (its Recruitee adapter yields a form) — the
  dependency cannot run both ways. The real `sources.Client` satisfies `Transport`
  structurally, so the worker passes the same client the crawl uses.

## How it works
`cmd/ingest` enqueues a capture when `applyform.NeedsRequestCapture(source)`
(`cmd/ingest/store.go:266`). `cmd/capture-apply-form` runs `applyform.Run`, which delegates
the claim/process loop to `internal/outbox.RunPool` (runner.go:115-133): claim a leased
wave, fetch each posting's form through its provider fetcher, and `Save` — which stores the
form and retires the queue entry atomically, so a capture cannot be both stored and left
queued. Tunables: `APPLY_FORM_BATCH_SIZE` (200), `_LEASE_SECONDS` (300, floored to the call
timeout so an in-flight capture is never re-claimed), `_MAX_ATTEMPTS` (3), `_CONCURRENCY`
(4), `_MAX_PER_RUN` (5000), `_CALL_TIMEOUT_SECONDS` (20) — internal/config/applyform.go:29-53.
