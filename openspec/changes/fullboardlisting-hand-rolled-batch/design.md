## Context

See proposal.md - Why. All six adapters already had a page-cap safety constant and an `added == 0`
(or source-declared-total) natural-end check; only the failure handling around a later page and
around the cap itself needed to change. Each adapter differs slightly in what proof of
completeness it already had available:

- `hh`, `peopleforce`, `gusto`: only a genuinely empty page proves completeness.
- `neogov`, `edjoin`: a genuinely empty page OR reaching the source's own stated total both prove
  it (`job-postings-number` for neogov, exact `totalRecords` for edjoin).
- `workstream`: a stated, in-range `totalPages` reached is itself proof (the source's declared
  page count), independent of whether the last page happened to be full; absent a valid stated
  count, only a genuinely empty page proves it.

## Goals / Non-Goals

**Goals:**
- Make each of the six adapters fail `Fetch` loudly instead of silently truncating, exactly when
  they cannot structurally prove the walk reached the board's end.
- Register each with the `fullBoardListing` marker.

**Non-Goals:**
- Raising any adapter's page-cap constant. No live measurement in this change shows any of the six
  boards is actually being truncated by its existing cap today — the fix is to the failure mode
  around the cap, not the cap's size. (Contrast `teamtailor-listing-cap-fix`, which raises
  `ttMaxPages` on the strength of a confirmed live probe.)
- Touching `bayt` (no source-declared total, already fragile to throttling — needs its own,
  slower-paced change) or `teamtailor` (already in flight elsewhere).
- Changing any adapter's transport, dedup key, or extra stop condition beyond the error handling
  described here.

## Decisions

**Track "reached a genuine end" with an explicit `done bool` rather than inferring it from the
loop's exit state.** A `for` loop that exits because it ran out of iterations looks identical, at
the call site, to one that `break`s early — the only way to tell "the walk proved completeness"
apart from "the walk gave up at the cap" is a variable the loop sets on the proof path and checks
after. This mirrors the pattern `taleo.go` and `careerplug.go` already use for the same reason.

**`workstream` seeds `done` from `stated` before the loop starts, rather than only setting it on
`added == 0`.** Its declared `totalPages`, when valid, is itself a proof of completeness
independent of whether the walk happens to see an empty page — the source is telling the walk
exactly how many pages exist, and reaching that count IS the end, full last page or not. Requiring
an empty page on top of a stated count would be requiring a THIRD proof the bar does not ask for.

**`neogov` and `edjoin` keep their `total > 0 && len(x) >= total` early-stop as an equally valid
proof, alongside the empty-page check, rather than picking one.** Both are legitimate under the
bar's first clause ("verify a fetched count against the source's own reported total"); neither
adapter loses information by accepting either signal, and requiring the empty page as well would
cost each of them one wasted request per crawl for no correctness benefit.

**Error messages name the board/role/job-type and the page or page-cap value, not just "failed."**
Matches every existing hard-fail message in this package (`teamtailor`'s "reached the %d-page
safety ceiling", `taleo`'s "still yielding requisitions after the page cap") — an operator reading
a failed-run log needs to tell a genuine upstream outage apart from "this adapter's cap needs
raising" without opening the source.

## Risks / Trade-offs

- **A previously-tolerated transient later-page hiccup now fails the whole board's crawl for that
  run**, where before it silently returned a partial list. This is the intended trade the
  `fullBoardListing` marker exists to make: a failed run is retried the next cron cycle (hourly for
  most of these adapters) and costs nothing but latency; a silent partial success reported as
  healthy is what let solidjobs close 110 live postings undetected. → Mitigation: none needed
  beyond what the marker already buys — `board_health`'s cooldown/backoff (see
  `ingest-board-health`) absorbs a board that fails repeatedly, and a single-run hiccup simply
  retries clean next cycle.
- **A board that has genuinely grown past today's page-cap constant now hard-fails instead of
  silently truncating.** This is strictly better than the prior silent truncation (a hard failure
  is visible in `board_health` and run logs; a silent partial success is not), but it does mean a
  board that crosses the threshold needs a human to raise the constant, the same way
  `teamtailor-listing-cap-fix` did for teamtailor. → Mitigation: none of the six caps are being
  raised speculatively in this change; if one starts failing in production, that failure IS the
  live-measurement signal the next fix should be based on — exactly the discipline
  `teamtailor-listing-cap-fix` modeled.
