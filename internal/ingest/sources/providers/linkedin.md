**LinkedIn traps** (`linkedin`; public HTML sampled 2026-11-13):

- Board is a search keyword (e.g. `software engineer`), Region is a required search location
  (e.g. `United Kingdom`); Company is a catalog label, never the posting's employer.
  The adapter reads the guest listing newest first, defaulting to the recent seven-day
  window and at most ten pages. Catalogue `tenants.pages` accepts 1..100 and `tenants.jobage`
  accepts 1..30 days. Explicit values must be unsigned decimal integers within these ranges;
  invalid options fail before any request. Missing keys retain ten pages and seven days.
  Optional `tenants.easyapply_pages` (0..100, default 0 = off, same strict validation) adds
  a second listing of the same search with `f_AL=true` (Easy Apply) after the primary
  pages, with its own offset, deduplicated against every id already listed. Its cards take
  exactly the primary path (seen-set, then detail read): `f_AL` is only a hint for where to
  spend detail requests, and the detail page alone decides the application method. Measured
  2026-11-17 on the live guest API: with `f_AL=true` 6 of 6 cards had the on-site apply
  control, without it 1 of 8 (7 off-site). An empty first Easy Apply page is not an error and
  a refusal keeps earlier progress. 40 pages is recommended. The completion line logs `easyapply_listed`.
  Raw card counts advance
  the listing offset, so a page limit is not a fixed posting count. Each completed search
  logs its configured page limit/age, primary listed/read counts and returned count including
  bounded recommendations; these are not publication or unique-growth figures.
  One limiter across the adapter's boards and both request paths allows one request
  every two seconds by default; `LINKEDIN_REQUEST_INTERVAL` (100ms..10s, set in
  `ingest-env/linkedin.env`) changes it, and a bad value fails the crawl. Requests leave
  through the rotating proxy, a new exit IP each, so lower it in steps while watching
  403/429/999 refusals.
- Listing URNs identify postings; `/jobs-guest/jobs/api/jobPosting/<id>` supplies employer, title, location and
  sanitized body (verified 2026-11-14; canonical numeric URLs returned 999). A stored posting is re-read only when the seen-set offers it again (`FetchNew`): a listed identity already stored is emitted as an identity-only `SeenRefresh` and costs no detail request, so the budget goes to new postings. The seen-set is provider-wide and keyed by the bare numeric id (`pipeline.seenScope`/`seenKey`, derived from `jobIdentity`), and withholds a row whose body is older than `BODY_REFRESH_DAYS`, which is what makes a stored posting re-read (its application method can change) every few days. `cmd/ingest` defaults linkedin to `BODY_REFRESH_DAYS=3` with slice 1 when the variable is unset (`sweep.WithProviderDefault`; an explicit value in `ingest-env/linkedin.env` wins; setting `BODY_REFRESH_DAYS` alone leaves `BODY_REFRESH_SLICE` at its default 30, so a stale posting waits for its slot in a 30-run cycle, and a three-day cadence needs `BODY_REFRESH_SLICE=1` as well), because a touch keeps `last_seen_at` fresh and `cmd/liveness` only probes stale rows: a posting closed on LinkedIn but still listed would otherwise stay open for good. A re-read that shows the closed notice drops the touch, so `last_seen_at` ages and liveness probes it. A touch never reopens a row closed `native_source_pending` (`TouchJob`): only a fresh detail read proving on-site Apply publishes it. Postings read as external or unknown are never stored as rows, so `linkedin_postings_read` (migration 0283: bare posting id, method, read_at) is their seen-set: ingest records one after native resolution succeeds (a failed resolution stays unrecorded and is retried), and the provider-wide seen lookup adds every id read since the body-refresh cutoff (the same 3-day default when unset). Such a posting is emitted as a `SeenRefresh`; touching it has no row and is a no-op. The lookup prunes rows older than 30 days, so no worker owns the table. A board whose listed postings are all seen is healthy. Missing details emit
  `Unreadable`; a wholly unreadable board fails. A 429 on any request is retried twice after
  10s and 20s pauses (the rotating proxy usually throttled one exit: on 2026-11-17 refusals
  clustered within seconds and stopping on the first ended searches after 7-64 of ~100 postings);
  a persistent 429, a 403 or a 999 stops the board.
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
  A transaction-scoped lock for that numeric posting is taken before identity lookup or
  posting/activity locks, so concurrent worker and contribution writes cannot race the
  public-slug unique index. Different posting ids do not share this gate.
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
