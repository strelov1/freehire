## Context

See proposal.md - Why. All six audited adapters already had a page-cap safety constant and a
cross-page-dedup natural-end check; the failure handling around a later page and around the cap
itself needed to change for five of them, plus a per-posting detail-drop gap for one. Each
adapter differs slightly in what proof of completeness it already had available:

- `hh`, `peopleforce`, `gusto`: only a genuinely empty page proves completeness.
- `neogov`, `edjoin`: a genuinely empty page OR reaching the source's own stated total both prove
  it (`job-postings-number` for neogov, exact `totalRecords` for edjoin).
- `workstream`: a stated, in-range `totalPages` reached is itself proof (the source's declared
  page count), independent of whether the last page happened to be full; absent a valid stated
  count, only a genuinely empty page proves it.

`hh` is the odd one out: live verification (curling hh.ru directly against professional_role 96,
one of freehire's own configured hh boards) showed its "genuinely empty page" proof is
unreachable for a busy role in the first place — hh.ru's own search UI refuses to paginate past
its own ~2000-result depth limit regardless of a role's true count, and role 96 alone has 6,657
results in the adapter's 7-day window. This is not a truncation this adapter's code causes or
could fix by raising a constant; it is an external ceiling the source itself enforces. See
Decisions.

## Goals / Non-Goals

**Goals:**
- Make each of the five qualifying adapters (`neogov`, `edjoin`, `workstream`, `peopleforce`,
  `gusto`) fail `Fetch` loudly instead of silently truncating, exactly when they cannot
  structurally prove the walk reached the board's end — proving emptiness from the RAW per-page
  item count, not the count of items newly kept after cross-page dedup.
- Register each of the five with the `fullBoardListing` marker.
- Close `peopleforce`'s per-posting detail-drop gap so a transient detail failure cannot silently
  remove an already-known posting from a run now trusted as a full-board listing.
- Audit `hh` with the same rigor and record why it does NOT qualify, rather than silently
  skipping it.

**Non-Goals:**
- Raising any adapter's page-cap constant. No live measurement in this change shows any of the
  five qualifying boards is actually being truncated by its existing cap today — the fix is to the
  failure mode around the cap, not the cap's size. (Contrast `teamtailor-listing-cap-fix`, which
  raises `ttMaxPages` on the strength of a confirmed live probe, and contrast `hh`, where the live
  measurement shows raising the cap would not even help — the ceiling is the SOURCE's, not the
  adapter's.)
- Touching `bayt` (no source-declared total, already fragile to throttling — needs its own,
  slower-paced change) or `teamtailor` (already in flight elsewhere).
- Changing any adapter's transport, dedup key, or extra stop condition beyond the error handling
  and the raw-count/detail-drop fixes described here.

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

**The empty-page proof reads the RAW per-page item count, before cross-page dedup — never the
count of items newly added to the result.** A page can be non-empty yet add nothing new: a sort
tie spanning a page boundary, or a source re-serving an already-seen page for an out-of-range
request. Stopping on "nothing new" in that case is unproven — a genuinely unseen posting could sit
on a page beyond it. Every one of the five adapters carries a regression test that serves a
duplicate-only non-empty page and asserts the walk still reaches a later, genuinely new posting.
This is a stricter reading of the marker's own bar (structural proof, not a heuristic that happens
to usually work) and was not part of the original mechanical pass — found on review.

**`hh` is audited and excluded, not silently left alone.** The original draft of this change
included `hh` in the marked set on the same mechanical reasoning as the other five. Live
verification against a real, currently-configured board (professional_role 96) showed that
reasoning does not hold: hh.ru's own search UI, not this adapter's code, is what stops pagination
at ~2000 results, and a role whose true count exceeds that (6,657 for role 96 in the 7-day window)
will hit that ceiling on every single run — a structural, permanent condition, not evidence of a
truncated crawl. Marking `hh` would have meant either failing this board's ingest on every run
(if the cap-exhaustion were made a hard failure, matching the other five) or granting the marker
without genuine proof (if it were not) — both wrong. `hh` keeps its original soft behavior
(`crawl` returns what it gathered when it exhausts `hhMaxPages`) and does not implement
`fullBoardListing`; its own code comment carries the measurement so a future reader does not have
to re-derive why.

**`peopleforce`'s `detail()` gains the established `unreadableDetail` marker rather than a new
mechanism.** `internal/ingest/sources/AGENTS.md` already documents this exact pattern — a detail
request that failed and a posting the platform says is gone are different answers, and an adapter
whose detail is a posting's only source must not spell them the same way — for eight other
non-hydrating link-only adapters (jazzhr, icims, careerplug, jobvite, successfactors, breezy,
bamboohr, smartrecruiters). `peopleforce` has the same shape (no `HydratingSource`, so every crawl
re-fetches every listed posting's detail) and, before now, simply was not part of that group
because nothing depended on its Fetch being complete. Now that it carries `fullBoardListing`, the
same reasoning applies verbatim: reusing `detailUnreadable`/`unreadableDetail` (careerplug.go's
`detail` is the closest structural match — a listing-plus-detail two-step, same as peopleforce) is
the idiomatic fix, not a new one invented for this adapter. Found on review, not in the original
mechanical pass.

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
  `teamtailor-listing-cap-fix` did for teamtailor. → Mitigation: none of the five caps are being
  raised speculatively in this change; if one starts failing in production, that failure IS the
  live-measurement signal the next fix should be based on — exactly the discipline
  `teamtailor-listing-cap-fix` modeled. `hh` is the one adapter in the original audit where this
  mitigation does NOT hold (its ceiling is the source's, not a margin we chose), which is exactly
  why it is excluded rather than hardened.
