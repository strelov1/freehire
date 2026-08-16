## Context

Measured on the prod host on 2026-08-16, immediately after the SEEK adapter shipped:

- One unpaced `cmd/ingest sources/seek.yml` run issued **3,267 GraphQL POSTs in 95 seconds** across
  43 boards, each board's detail pool bursting to `defaultDetailWorkers` (8).
- SEEK answered **429** to essentially all of them; the run ingested 2,982 postings of which 2,593
  (87%) have no description.
- A single identical request from the same host returned **200 after two minutes idle** — so this is
  a burst window with a short penalty, not an IP block. The search listing (~150 requests a run) was
  never refused, before or after.

## Goals / Non-Goals

**Goals:**

- Hold a whole run's detail-request rate under SEEK's window, regardless of how many boards hydrate
  at once.
- Guarantee that a refused detail costs a delay, never a permanently body-less catalogue row.

**Non-Goals:**

- Pacing the search listing. It is ~150 requests a run and has never been refused.
- Fixing the hydration contract for every `HydratingSource`. The `seen` predicate carries only row
  existence, so a general repair means new SQL and a new pipeline capability shared by ten adapters —
  a change worth making on its own evidence, not smuggled into a hotfix.
- Proxy egress. The endpoint answers this IP fine when asked politely; a proxy would mask a pacing
  bug rather than fix it.

## Decisions

**Pace, do not merely cap concurrency.** `pacer.go` offers both levers and is explicit about when
each fits: a semaphore is right for an API that degrades under *sustained concurrent load*
(trudvsem, emagine), a rate limiter for one that enforces a *request budget per window*. SEEK
returned an immediate 429 and recovered on idle rather than degrading — that is a budget, so the
limiter is the matching tool. Capping in-flight requests alone would not help: 8 fast requests a
second from one worker is still 8 a second.

**One limiter per registry build, shared across every board.** Same construction as
`pacedHTMLGetter` and `limitedEmagineGetter`. A per-board limiter would multiply the rate by 43.

**The transport role is new.** The existing pacer wraps `HTMLGetter` and `JSONGetter`; SEEK's detail
is a JSON **POST**, so this adds a paced `JSONPoster`. Only SEEK's detail path is wrapped; its
listing keeps the bare client.

**Rate: 2 requests/second (500ms interval, burst 2).** Chosen the way every other constant in this
file was — conservative, because the true budget is unknown and the file's own guidance is that
under-shooting costs run length while over-shooting re-enters the penalty. It is ~17x slower than
the rate that was refused. Consequence: the ~7.8k first backfill needs ~65 minutes and so does NOT
fit one ingest window (`TimeoutStartSec=3000`, ~2400s of crawl budget), accreting over roughly two
to three runs instead. Steady state is a few hundred new postings a day and finishes in minutes.

**Drop a posting whose detail failed, rather than ingest it body-less.** This reverses the rule the
adapter shipped with and that `hh` and the other hydrating adapters keep. That rule is sound where
its premise holds — detail failures are rare, so keeping the posting beats losing it. SEEK breaks the
premise: refusals arrive in bursts of thousands, and because `seen` reports only row existence,
every body-less row it creates is permanent. Deferring a posting by one crawl is recoverable; storing
it body-less is not. The asymmetry decides it.

*Alternative considered:* teach the seen-set to exclude description-less rows, repairing all ten
hydrating adapters. Correct, and out of scope for a fix to a live data-quality fault — recorded as
the follow-up it deserves.

## Risks / Trade-offs

- **The rate is a guess** → under-shooting only lengthens the backfill; the drop-and-retry rule makes
  an unfinished run safe. Tune from the observed description-fill rate, as the file's other constants
  instruct.
- **A posting is briefly absent rather than present-but-empty** → accepted, and the better failure:
  the catalogue never shows an empty vacancy, and the next crawl repairs it.
- **The general hydration hole remains for the other nine adapters** → unchanged by this fix, now
  written down as its own problem rather than a footnote in SEEK's design.

## Migration Plan

Ship the adapter change, then `cmd/prune` the existing `seek` rows (it archives to `pruned_jobs`
rather than losing them), then run the paced crawl and confirm the description-fill rate before
enabling the hourly timer.

## Open Questions

Whether 2 req/s is near SEEK's real budget or far under it. The first paced run's refusal count
answers it.
