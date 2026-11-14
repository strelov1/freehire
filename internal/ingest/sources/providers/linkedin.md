**LinkedIn traps** (`linkedin`; public HTML sampled 2026-11-13):

- Board is a search keyword (e.g. `software engineer`), Region is a required search location
  (e.g. `United Kingdom`); Company is a catalog label, never the posting's employer.
  The adapter reads the guest listing, recent seven-day window, newest first, at most ten
  pages. One limiter across the adapter's boards and both request paths allows one request
  every two seconds. This is an initial conservative budget, not a measured sustained limit.
- Listing URNs identify postings; `/jobs-guest/jobs/api/jobPosting/<id>` supplies employer, title, location and
  sanitized body (verified 2026-11-14; canonical numeric URLs returned 999). Seen postings take the normal `SeenRefresh` path. Missing details emit
  `Unreadable`; a wholly unreadable board fails. A detail refusal stops the board.
- The bounded listing never supplies a quiet-board fingerprint, including on partial
  progress after later-page errors; an unchanged prefix must not delay its next crawl.
- The listing is incomplete even for employers it names: `probeClosing` excludes ALL unseen
  sweeps and the chronic/empty-feed/board-gone safety nets, and `livenessProbed` excludes search's unseen timeout. `cmd/liveness` checks stale
  postings at one request per two seconds, at most 300 per run, stopping on 403/429/999.
- A primary closed notice is the only supported death signal; HTTP 200 alone is not live,
  and even 404/410 remain uncertain until a larger sample validates them. A complete page
  with the primary Apply control is live. Final and canonical URLs must match the native ID.
  Conflicting signals and login pages are uncertain. Closure takes the existing two strikes.
  Rediscovery reopens through ingest; the worker does not periodically revisit closed rows.
- Add boards with `cmd/add-board --provider=linkedin --board='software engineer'
  --region='United Kingdom' --company='LinkedIn UK software' --apply`, then arrange the
  normal ingest and liveness schedules in freehire-ops. Adapter registration alone starts
  no crawl. A LinkedIn closure does not close an independent employer ATS copy.
