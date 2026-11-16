**LinkedIn traps** (`linkedin`; public HTML sampled 2026-11-13):

- Board is a search keyword (e.g. `software engineer`), Region is a required search location
  (e.g. `United Kingdom`); Company is a catalog label, never the posting's employer.
  The adapter reads the guest listing, recent seven-day window, newest first, at most ten
  pages. One limiter across the adapter's boards and both request paths allows one request
  every two seconds. This is an initial conservative budget, not a measured sustained limit.
- Listing URNs identify postings; `/jobs-guest/jobs/api/jobPosting/<id>` supplies employer, title, location and
  sanitized body (verified 2026-11-14; canonical numeric URLs returned 999). Every listed posting is re-read, including previously seen identities: its application method can change. Missing details emit
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
- Only the explicit `public_jobs_apply-link-onsite` control establishes LinkedIn-native
  application. The off-site control or its external-arrow icon establishes external Apply;
  generic Apply and conflicting controls remain unknown. External and unknown discoveries
  never enter the public catalogue under the LinkedIn source.
- External destinations resolve through the native ATS adapter and use its ordinary
  `(provider, board:native-id)` identity. `cmd/ingest` onboards the board, transfers an
  existing LinkedIn row when the native employer/title agree, and preserves its id, public
  slug and history. An already stored native identity stays canonical; the old discovery
  copy closes through the existing `ats_covered` policy. Conflicts are never merged blindly.
- Guest pages sampled 2026-11-16 hide external Apply addresses behind sign-in. Reuse the
  public company profile's Organization.sameAs and follow observed employer careers links
  through the shared board resolver. Never guess ATS hosts from employer names. A discovered
  board is pending until the normal ATS ingest proves it; no nested whole-board crawl runs
  inside the LinkedIn worker. New providers join managed scheduling on existing defaults;
  existing static providers and operator pauses keep their ownership/settings.
- Reconcile legacy rows with `ingest linkedin --reconcile-linkedin`. It directly checks up
  to 300 stored identities; use the reported `--reconcile-after` cursor for another batch.
  Unresolved rows are reported as pending and exit nonzero. A verified curator destination
  can be transferred with `cmd/promote-job-source --slug=... --url=... --apply`; merely setting
  `application_url` does not change ownership. LinkedIn disappearance never closes the ATS.

- Public full posting pages can expose `section.similar-jobs` although the guest detail
  fragment has none. Sample `4490679811` on 2026-11-16 contained 27 cards, including the
  entries initially hidden behind "Show more jobs like this"; 15 predated the search's
  seven-day window. After a complete normal listing, fetch one full companion page for
  the first readable live posting and verify its primary status, final identity and
  canonical metadata before reading recommendations. Admit only numeric posting URNs
  with a matching LinkedIn job link inside this section, deduplicating against every
  primary listing page. Hydrate at most 20 additional identities through the ordinary
  guest reader, preserving application-method classification and native ATS handoff.
  Depth is one: recommendation pages never seed another full-page read. All requests
  share the adapter's existing limiter. A companion failure preserves the primary crawl;
  a recommendation refusal stops that branch and preserves earlier readable jobs.

- New catalogue identities use the numeric LinkedIn posting id, independent of keyword or
  country. The shared job upsert reuses any existing public keyword-scoped
  row with the exact normalized native URL, preserving its id, public slug and history.
  Cheap refresh and ATS-coverage retirement map numeric discoveries to those legacy keys.


- On 2026-11-16, the crawl host's existing Scrapling HTTP fetcher with its proxy and
  Chrome fingerprint read full public postings `4472812273` and `4490679811` with correct
  canonical identities and 27 related cards each. LinkedIn therefore uses this existing
  HTTP+proxy route when the Scrapling service is configured; `NewLinkedIn` retains its
  one shared request limiter across search, details, profiles and recommendations.
  The bounded probe establishes access, not sustained crawl coverage. External ATS
  destinations remained hidden; changing User-Agent to Mobile Safari added no ATS link
  and requires no new service option. Application-method and native-employer proof
  remain mandatory. Without Scrapling configuration the prior transport is retained.
