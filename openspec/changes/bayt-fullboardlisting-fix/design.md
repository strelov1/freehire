## Context

See proposal.md - Why. `bayt` was excluded from `fullboardlisting-hand-rolled-batch` on two stated
concerns: no source-declared total (only an empty-page proof is available), and a documented,
live-observed throttling risk. The first concern is not actually a blocker — several of the six
batch-fixed adapters (`hh`, `peopleforce`, `gusto`) also had only an empty-page proof available and
were fixed the same way. The second concern needed investigation before a decision either way.

`bayt.go`'s own comments distinguish two separate risks: the base Akamai/Cloudflare TLS-fingerprint
block (which 403s Go's default transport regardless of pacing, and is already solved in production
by wiring the shared Chrome-fingerprint transport) and a SEPARATE, narrower throttling risk tied
specifically to a fast CONCURRENT burst against the detail fan-out (`baytDetailWorkers = 3`, already
narrowed below the shared `defaultDetailWorkers = 8` for exactly this reason). The listing walk
(`Fetch`'s page loop) is single-threaded — one request at a time, sequential — structurally
different from the concurrent detail fan-out the throttling comment was written about. There is no
code, comment, or measurement suggesting the sequential listing walk shares that risk.

## Goals / Non-Goals

**Goals:**
- Bring `bayt` to the same `fullBoardListing` bar the six batch-fixed adapters already meet:
  fail `Fetch` loudly instead of silently truncating when completeness cannot be structurally
  proven, using the raw-count fix already established for the batch.
- Close `bayt.detail`'s per-posting drop gap — it has no `HydratingSource`, so every crawl
  re-fetches every listed posting's detail, and it dropped a posting outright on ANY fetch
  failure — using the same `unreadableDetail` pattern `peopleforce` already got.
- Resolve the throttling concern that excluded `bayt` from the original batch, rather than leaving
  it open indefinitely: distinguish the concurrency-specific risk (detail fan-out) from the
  sequential listing walk, and confirm the fix does not conflate the two.

**Non-Goals:**
- Raising `baytMaxPages`. No live measurement shows any real country's listing reaching it.
- Widening `baytDetailWorkers` or otherwise touching the concurrency pacing that already exists
  for the documented throttling risk — this change only changes what happens when a request
  (listing or detail) FAILS, not how many run at once.
- Re-litigating the base Akamai/Cloudflare TLS-fingerprint transport choice (`fingerprintHTTP`),
  which is unrelated to this change and already solved.

## Decisions

**Grant `bayt` the marker rather than leaving it permanently excluded.** The throttling concern
that excluded it from the original batch was never actually measured against the listing walk
specifically — it was inherited caution from the detail fan-out's own documented risk. Structurally,
the listing walk is a different, lower-risk code path (sequential, not concurrent), and the same
mitigation already accepted for the six batch-fixed adapters (`board_health`'s cooldown/backoff
absorbs an occasional transient failure) applies here identically. Leaving `bayt` excluded on an
unexamined worry, after having examined it, would not be honest caution — it would be a stale
assumption.

**The raw-count empty-page proof filters to job-shaped links first, unlike the batch's simpler
cases.** `bayt.Fetch` already collects EVERY anchor href on a page via `baytListingLinks` (title
links, apply buttons, navigation, footer) and only classifies job-detail links afterward via
`baytJobID`. Using the raw anchor count directly (mirroring how `workstream`/`gusto`/`peopleforce`
read their already-job-shaped parsed cards) would never be zero — a listing page's navigation chrome
alone guarantees a non-empty raw anchor list even past the board's real end. The proof therefore
has to be the raw count of anchors that ARE job-detail links (post-`baytJobID`-filter, pre-dedup),
not the raw count of every anchor on the page.

**`detail` gains an `e CompanyEntry` parameter to carry the configured entry's company into
`unreadableDetail`'s marker.** Every other `unreadableDetail` call site in this package already has
its `CompanyEntry` in scope; `bayt.detail` previously did not need it (the real employer comes from
each posting's own `hiringOrganization`, never the configured entry) so it was never threaded
through. The marker still needs SOME company name to scope a close-window withholding decision to,
and `e.Company` — "the board's configured employer when nothing better is known", per
`unreadableDetail`'s own doc comment — is the established fallback for exactly this situation.

## Risks / Trade-offs

- **[Risk]** The sequential-vs-concurrent distinction this change relies on is reasoned from the
  existing code's own structure and comments, not from a fresh live measurement of the listing
  walk's throttling behavior specifically (Bayt's Akamai edge blocks this sandbox's plain `curl`
  on TLS-fingerprint grounds regardless of request pacing, so the listing-walk-specific throttling
  question could not be isolated by a live probe the way `hh`'s and `teamtailor`'s decisions were).
  → **Mitigation**: if the listing walk turns out to throttle more than expected, that will surface
  as `board_health` failures on `bayt`'s boards — visible, not silent — and is the same fallback
  the design for the six batch-fixed adapters already leans on. The detail-fan-out's own pacing
  (`baytDetailWorkers`) is untouched by this change, so the risk this change COULD make worse
  (concurrent throttling) is not the risk this change actually touches (sequential listing
  failures, and detail-fetch DROPS, which `unreadableDetail` now prevents rather than causes).
- **[Trade-off]** Same as the six batch-fixed adapters: a previously-tolerated transient later-page
  hiccup now fails the whole board's crawl for that run. → Accepted, same mitigation
  (`board_health` cooldown/backoff).
