## Why

Layer 1 (`source-quality-metrics`) counts what each source leaves MISSING — no work mode, no
place, no employer, unseen for a week — from one grouped scan every three hours. It cannot say
what is WRONG: a posting we show that its source has already taken down, or a posting we tag
`remote` whose own text says "three days a week in our Berlin office". Both are accuracy
questions, and a counter over our own columns has no way to answer them; the only evidence is
outside the row — the source's page, and what the posting's text actually says.

The two are also the failures a visitor feels most directly. A dead posting costs them an
application into the void; a false `remote` costs them the search they filtered by. A manual
audit on 2026-10-29 found only 62% of a sample of "remote, worldwide" postings honest — a number
nothing in the pipeline would ever have reported.

## What Changes

- A new weekly run-once worker, `cmd/quality-audit`, that MEASURES and never acts:
  - **Liveness.** For every source with at least 500 open public postings (read from Layer 1's
    `source_stats`), probe a random sample of its postings' own URLs and count how many the
    source says are gone (404/410) and how many it says are live (2xx, not redirected away). A
    verdict counts only when the host has first been shown to answer 404/410 for a posting id
    that cannot exist (a "canary"), so a source that answers 200 for anything — or sits behind
    a bot wall — is reported as unmeasured, never as 0% dead. Sources whose closed postings are
    known to answer 200 are declared unmeasurable outright. robots.txt and Crawl-delay are
    honoured, each host is paced, a 403/429 ends that host, and the whole run has a request
    budget.
  - **Remote accuracy.** For a bounded sample (~200 a week) of postings tagged
    `work_mode = 'remote'`, an LLM judge reads title, location and description and answers
    remote / hybrid / onsite / unclear, plus whether the posting is open worldwide. An answer
    must quote the posting verbatim or it is demoted to `unclear`. Runs on the SERVICE
    credential under `feature: quality-audit`. Judgements are persisted, because ~20 postings
    of a source in one week is not a precision; the published figures cover a trailing 8 weeks.
- Per-source gauges through the node_exporter textfile collector: raw counts only, the alert
  rule owns every threshold.
- A small table, `remote_audit_judgements`, holding each judgement (migration).

## Capabilities

### Modified Capabilities

- `source-quality-metrics`: adds the sampled accuracy layer — liveness and remote precision.

## Impact

- New: `cmd/quality-audit`, `internal/job/qualityaudit`, a migration creating
  `remote_audit_judgements`, `internal/platform/db/queries/quality_audit.sql`.
- Ops (freehire-ops, not this repo): a weekly unit + timer on the database host, and alert rules.
- Cost: at most 15k outbound HTTP requests a week, spread over ~300 sources' hosts; ~200 LLM
  calls a week (about 0.4M input tokens).
