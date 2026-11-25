## Context

Layer 1 publishes `freehire_source_quality_gap{source,check}` from `source_stats`, a snapshot
`cmd/rollup-stats` rebuilds every three hours. It counts empty columns. This change adds the
layer that checks columns against the world, by sampling, weekly.

Two lessons from the same week shape the liveness half:

- **Status is trustworthy; bodies are not.** The shared body classifier
  (`internal/job/liveness.Classify`) read arbeitnow's live `.ch` pages as expired because they
  say "no longer accepting applications from the UK". A 404/410 from the source and a 2xx that
  stayed on the posting's own URL are the only verdicts this audit reads.
- **Some sources lie with their status.** jobtech (Platsbanken) and whatjobs' CPC landing
  answer 200 for any id; adzuna, wellfound and himalayas sit behind bot walls reachable only
  through the Scrapling tier on the crawl host; hh, djinni and torre keep a CLOSED posting at
  200 with a banner (see `cmd/liveness`'s per-source passes). A sample of those read naively
  would report 0% dead — the most confidently wrong number possible.

## Goals / Non-Goals

**Goals:** a weekly, bounded, polite measurement of (1) the share of shown postings that are
dead at the source and (2) the precision of the `remote` tag (and of `global` among them), per
source, as raw-count gauges. A source the audit cannot measure must SAY so.

**Non-Goals:** closing anything (`cmd/liveness`, `cmd/adzuna-liveness` and the sweep own
closes); fixing tags; reaching bot-walled sources through Scrapling (the seam is noted below);
alert thresholds (they live in freehire-ops).

## Decisions

### One worker, two passes, `internal/job/qualityaudit`

`cmd/quality-audit` runs both checks and writes one textfile, `freehire-quality-audit.prom` —
not `<binary>.prom`, which `worker.Main` overwrites with the run outcome (the same reason
`cmd/queue-metrics` publishes as `freehire-pipeline.prom`). The domain logic (sampling verdicts,
the canary, robots, the judge prompt and its grounding) lives in `internal/job/qualityaudit`:
it is about a posting's reality, beside `job/liveness` and `job/ghost`, and needs nothing above
`platform`.

Without `PROM_TEXTFILE_DIR` the worker is a no-op that never opens the pool, like every other
publisher. `-dry-run` is the exception: it runs both passes, writes nothing (no judgement row,
no textfile) and prints the exposition — the way to try it by hand.

### Liveness: the canary is what makes a verdict count

For each sampled posting the evidence is one GET of its stored URL, read by status only:

- 404 or 410 → **dead**.
- 2xx whose final URL is the requested one (same host, same path; a trailing slash and the
  query are ignored) → **live**.
- anything else → **unmeasured**, with a reason (`no_answer`, `moved`, `refused`, …). A 2xx
  that redirected elsewhere is `moved`, not live: greenhouse sends a closed posting to its
  board page with a 200. Redirects are followed only while they stay on the posting's host and
  on paths its robots.txt allows (at most 5 hops); one that leaves is handed back unfollowed and
  is `moved` too — robots and pacing were checked for this host, not for wherever it points.

A dead/live verdict is only kept when the posting's HOST is calibrated: before reading any of a
host's postings, the audit asks it for a posting that cannot exist — the stored URL with the
posting id (the last `:`-segment of `external_id`, at least 5 characters, found in the URL)
replaced by a random token of the same shape. Only a 404/410 calibrates the host. A host that
answers the canary 200 (jobtech's "any id"), 403 (a bot wall) or with a redirect is
uncalibrated, and every sampled posting on it is `unmeasured: uncalibrated` without being
fetched — the canary is also what keeps the request count down for a host that cannot answer.

The calibration is per host, not per source: a source's postings often live on employers' own
domains (greenhouse, workday tenants), and one canary on one of them says nothing about the
next. It is symmetric by construction — a host that cannot prove a 404 means "gone" gets
neither its dead verdicts nor its live ones counted — so an uncalibrated host can never inflate
or deflate the dead share, only shrink the measured count.

Considered and rejected: calibrating once per source (wrong for multi-domain sources);
reading bodies (the arbeitnow false positive); counting a 404 without a canary while requiring
one for "live" (biases the share toward dead on a host that walls the canary).

The canary does not catch a source that 404s an id that never existed but keeps a CLOSED
posting at 200 (hh, djinni, torre). Those are listed in `statusBlind` with the reason, and
are reported with every sample unmeasured. The list is a known lower bound: a source not on it
with the same behaviour would read as healthier than it is. The seam for doing better is the
per-source readers `cmd/liveness` already has (hh's archived flag, torre's page state); moving
them into a shared package is the follow-up if one of these sources needs a number.

Politeness: robots.txt is fetched once per host and honoured for the `freehire` agent (or `*`):
a disallowed path is `unmeasured: robots`, a Crawl-delay above the default pacing is used, one
above 60 s skips the host. A 5xx robots.txt disallows the host (RFC 9309); a 4xx allows it,
except a 403/429, which ends the host as refused. Up to 500 KiB of robots.txt is parsed.
Requests to one host are sequential and spaced (default 2 s); hosts run 16 at a time; a 403 or
429 from a posting ends that host for the run. Every request — robots, canary, posting — draws
on one budget (`QUALITY_AUDIT_MAX_REQUESTS`, default 15000); a posting reached after it ran out
is `unmeasured: budget`. Host order is shuffled so the budget's victims rotate.

The sample: every source with `source_stats.open_jobs >= 500` (Layer 1's snapshot, so the two
layers share a denominator), `QUALITY_AUDIT_LIVENESS_SAMPLE` postings each (default 30), open
and non-private, uniformly random (`random() < fraction` then `ORDER BY random()`, so the
sort is over a few dozen rows, not a source's whole catalogue). About 294 sources today:
~8.8k posting requests plus one canary and one robots.txt per host.

The liveness result is NOT persisted: it is a current-state reading and the next week replaces
it; the run log names every dead posting for a drill-down.

### Remote: a strict judge, grounded, persisted

The judge sees title, location and the description as text (truncated to 6000 runes) — never
our own tags, so it cannot anchor on them — and answers JSON:

```json
{"work_mode": "remote|hybrid|onsite|unclear", "work_mode_quote": "...",
 "worldwide": "yes|no|unclear", "worldwide_quote": "..."}
```

The prompt demands `unclear` whenever the text does not say, and a verbatim quote for any other
answer. The quote is checked: an answer whose quote is not in the posting (case and whitespace
folded) is demoted to `unclear` — the `companyname.GroundedName` rule, applied to a judgement.
A model that infers "remote" from a startup's vibe gets no credit.

The service credential, tagged `llm.Feature("quality-audit")` (`model.As("", nil, ...)`, the
`backfill-company-names` pattern), so the spend reads apart in the gateway. No user is involved;
the paid-AI gate does not apply to ownerless background work.

Sampling: ~204 sources carry 50+ open remote postings, so ~200 judgements a week cannot cover
them all evenly. Each run picks `QUALITY_AUDIT_REMOTE_MAX / QUALITY_AUDIT_REMOTE_PER_SOURCE`
sources (default 200 / 20 = 10), WEIGHTED by their open remote count without replacement, and
judges a uniform sample of each. Weighting puts the measurement where visitors see remote
postings; without replacement keeps manatal from taking the whole week. Each judgement is
stored in `remote_audit_judgements`, and the gauges are computed over the trailing 56 days, so
a large source accumulates dozens of judgements and a small one appears when it is drawn.

Considered: a uniform sample over all remote postings (manatal alone would take ~23 a week and
the long tail none); an equal share per source (n≈1 each — not a precision).

### Gauges

All raw counts; the alert rule owns thresholds and the minimum sample.

```text
freehire_audit_liveness_sampled{source}       postings sampled this run
freehire_audit_liveness_live{source}          calibrated, source says live
freehire_audit_liveness_dead{source}          calibrated, source says gone
freehire_audit_remote_sampled{source}         judgements in the last 56 days
freehire_audit_remote_confirmed{source}       ...the judge read "remote"
freehire_audit_remote_contradicted{source}    ...the judge read "hybrid" or "onsite"
freehire_audit_global_sampled{source}         ...of postings also tagged regions 'global'
freehire_audit_global_confirmed{source}       ...judge: open worldwide
freehire_audit_global_contradicted{source}    ...judge: restricted to some countries
freehire_audit_measured_timestamp_seconds{check}  when each check last ran
```

`sampled - live - dead` is the unmeasured share, and a source with `live + dead = 0` is
unmeasurable this week — the alert must not read it as healthy. `unclear` is `sampled -
confirmed - contradicted`. A source with no judgement in the window publishes no remote series
(absent, not zero).

## Risks / Trade-offs

- A canary id could collide with a real posting → that host reads as uncalibrated (coverage
  lost, never a wrong verdict).
- A host whose unknown ids 404 but whose closed postings redirect → those postings read
  `moved` (unmeasured): the dead share is a lower bound there, visible as a high unmeasured
  share.
- The judge is a model: precision of the remote tag is measured against its reading. Grounding
  and `unclear` keep it conservative; the 2026-10-29 manual audit (62% honest for global
  remote) is the sanity reference for the first runs.

## Migration Plan

Migration adds one table (no backfill). Deploy, then install the weekly unit/timer in
freehire-ops (unit text in the PR). Rollback: stop the timer; the table is inert.
