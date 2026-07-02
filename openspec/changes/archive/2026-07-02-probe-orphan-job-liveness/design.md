# Design — probe-orphan-job-liveness

## Context

`close-stale-jobs` made `closed_at` the single soft-close state and the ingest sweep
its writer. That sweep is structurally blind to jobs no crawl re-visits: it is
per-provider (`CloseUnseenJobs WHERE source = $provider`, called from `cmd/ingest`),
so a job whose `source` is not a registered ATS provider is never a candidate. Those
orphan sources are `telegram` (direct TG extraction, `cmd/tg-extract` store.go) and
the non-ATS link sources `habr_career` / `geekjob`. They are upserted once and live
forever.

The career-ops project solves posting liveness by probing the URL and classifying the
page; this change ports that idea (heuristics only, no Playwright, no LLM) and routes
its outcome into the existing `closed_at` model.

## Decisions

### Reuse `closed_at`; "expired" is a classification, not a state

To the catalogue and to users, a closed job is a closed job — the cause (ingest sweep
vs URL probe) is irrelevant. So liveness introduces no new lifecycle column and no
wire-shape change: `expired` lives only in the worker for the duration of a probe and,
on confirmation, performs the same `SET closed_at = now()` the sweep does. All
visibility logic (`closed_at IS NULL` filters, detail-serves-closed, reindex
deletion, the SPA closed notice) is inherited unchanged.

### Scope: orphan sources only, by exclusion from the ATS registry

The probe selects open jobs whose `source` is **not** in the registered ATS provider
set (`sources.All` keys). This is self-maintaining: it captures exactly the
never-swept sources (`telegram`, `habr_career`, `geekjob`, and any future non-ATS
source) and naturally excludes board jobs the sweep already owns. The exclusion list
is derived from the same registry the ingest pipeline validates against, so a new ATS
adapter never silently becomes a liveness target.

Out of scope: link-source `greenhouse`/`ashby` jobs share a `source` string with the
ATS adapters and are therefore already swept (possibly wrongly) by collision. That is
a pre-existing issue; this change neither relies on nor fixes it.

### Multi-signal `expired`, biased to under-close

A single HTTP status is not enough — most dead postings return 200 with a "no longer
available" body or redirect to a generic careers page. The classifier (`internal/
liveness`) returns `expired` on any definitive death signal:

- HTTP `404` / `410`;
- final URL matching an error/listing redirect pattern;
- body matching a curated hard-expired pattern (EN/DE/FR, e.g. "no longer accepting
  applications", "position has been filled");
- body content length below a small threshold (a nav/footer-only shell).

The classifier returns three verdicts, because the worker must act differently on
"alive" than on "could not tell":

- `expired` — the death signals above; advances the strike counter.
- `live` — a healthy `2xx` with no death signal; clears the strike counter.
- `uncertain` — a non-`2xx` that is not a definitive gone (`5xx`, `403`) or a
  network/timeout error; takes **no action**, leaving the strike count untouched.

The `live`/`uncertain` split matters: a transient `503` is not proof the posting is
alive, so it must not reset a strike already earned from a real `404` on the previous
run (otherwise a flaky host could keep a dead job open forever). A fetch error surfaces
as status `0`, which maps to `uncertain` — so the worker needs no special-case branch
for transport failures. Without a headless browser we lose the "visible Apply control"
signal and under-detect JS-only closures; that is an accepted trade-off that fails safe
(miss a dead job, never kill a live one).

### Two-strike grace via a counter column

Orphan jobs have no re-ingest path, so a false close is permanent — the policy must be
conservative. `jobs.liveness_strikes int NOT NULL DEFAULT 0` records consecutive
expired reads:

- `expired` → increment; when it reaches **2**, set `closed_at = now()` in the same
  `UPDATE`;
- not-expired → reset to 0.

Requiring two *consecutive* expired reads across separate runs absorbs a transient
404 (employer-site deploy) without a job→probe-history table. A flapping site (404,
then timeout, then 404) never accumulates two in a row and stays open — the
under-close bias again. This mirrors the time-window grace of the 48h sweep, expressed
as a run-count because the probe, unlike the crawl, has no "last seen" timestamp to
threshold against.

A counter is chosen over a `liveness_dead_since` timestamp because "two consecutive
confirmations" is the exact intent, and a timestamp would close on a single stale
reading once the window elapsed even without reconfirmation.

### Run-once worker, probe all candidates per run

`cmd/liveness` follows the established worker shape: load config, do the work once,
exit; safe to re-run; scheduled on cron beside ingest/enrich. It fetches via the
shared `internal/sources` HTTP client with a per-probe timeout and bounded
concurrency, classifies, and applies the strike/close/reset update per job. The close
reason (`http_gone`, `expired_body`, `expired_url`, `insufficient_content`) is logged,
not persisted.

At current orphan volume the worker probes every candidate each run. Round-robin
selection via a `liveness_checked_at` column is a noted seam for when volume grows;
adding it later changes only the candidate query.

## Risks

- **Search index drift**: a job closed by liveness between reindex runs stays
  searchable until the next reindex (≤6h) — the same accepted window the index already
  has.
- **Under-close**: JS-only closed pages and flapping dead sites are not closed. This
  is the safe failure mode and is preferred over false-closing un-reopenable orphans.
- **Probe politeness**: orphan postings span many hosts; bounded concurrency + a
  per-probe timeout keep the worker from hammering any single employer site.
