# ClinchTalent

Board is the career-site hostname. Read `/sitemap.xml`, retain `/jobs/` URLs and
use the UUID suffix or full slug as the external ID. Detail metadata does not change
identity, company ownership or sitemap timestamps.

On 2026-11-15, real Mader and Armaguard details exposed `.job-title` and
`.job-component-location` components. Prefer their text to slug guesses; preserve
punctuation, abbreviations and every explicit location. Other verified themes use
`.job-location` spans or `.job-component-location-group`; repeated responsive
copies are deduplicated. Physical locations take priority over location groups. A generic h1 can be page
chrome (Mader says `Careers`), and the document/OG title appends geography.
See the reduced real fixtures under `testdata/clinch/`.

Slug geography remains a best-effort fallback when explicit components are absent.
Do not adjust the location dictionary to compensate for ambiguous job-title words.
A detail challenge latches hydration off for the run; emit all sitemap jobs with
fallback metadata. Clinch implements `HydratingSource`: known hydrated IDs yield
`SeenRefresh`, while a failed, empty or latched detail yields `Unreadable`. Both retain
listing liveness without rewriting content or advancing `hydrated_at`; unreadable
postings remain pending and prevent quiet backoff; excessive unreadable coverage
withholds unsafe sweeps under the shared pipeline policy. New unreadable
IDs are emitted but are not persisted until a real body is available. Ordinary detail
errors do not latch or drop listing identities.

The shared seen-set retries recent persisted empty bodies and, when enabled, withholds
stale bodies in daily slices so employer edits still reach stored postings. Production
must enable `BODY_REFRESH_DAYS=30` / `BODY_REFRESH_SLICE=30` in the ops slot wrapper
for Clinch alongside this adapter change (preserving explicit overrides). Without it,
old empty rows beyond the hydration retry window and future body edits may remain
unread. Do not enable refresh by stamping or resetting stored activity. This adapter does not declare `fullBoardListing` or `emptyBoardListing`;
a sitemap-only run does not establish readable-body coverage or prove an employer
has no vacancies. Do not use such a run to justify retiring its former PageUp board.

With Scrapling configured, the registry keeps sitemaps on the direct client and
routes the 38 verified migrated hosts' details through HTTP + proxy beneath the same provider-wide limiter
(1500 ms, burst one). Thirty migrated targets needed this lane in the bounded
2026-11-15 sample; later cold direct successes do not establish sustained access.
Service errors and upstream refusals retain standard client handling and the
existing adapter fallback/latch. There are no extra outer retries or new budgets.
See [completion evidence](../../../../docs/investigations/2026-11-15-ingest-completion.md).
