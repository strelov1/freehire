## Context

Dover (app.dover.com) exposes a public, unauthenticated JSON API behind its React SPA career
pages. Verified live against `qompyl` (the board named in the triggering request):

- `GET /api/v1/careers-page-slug/{slug}` → `{id: <client_id uuid>, slug, name, primary_domain, ...}`
- `GET /api/v1/careers-page/{client_id}/jobs?limit=300&offset=0` → paginated
  `{count, next, previous, results: [{id, title, locations[], workplace_type, is_published, is_sample}]}`
  — no description.
- `GET /api/v1/inbound/application-portal-job/{jobId}` → full detail: title, `user_provided_description`
  (HTML), `locations[]`, `workplace_type`, `compensation{...}`, `visa_support`, `created`, `active`,
  `is_private`, `client_name`/`client_domain`, `application_questions[]`.

All three respond 200 to a bare `curl -A curl/8.0` — no Cloudflare challenge on the data API.
Cloudflare Turnstile only loads on the browser apply-submission flow, which this change does not
touch. `app.dover.com` is Dover's whole product host (dashboard, `/crm`, `/docs/api`, login, the
public career pages) — not a dedicated boards subdomain — which matters for the `atsboard`
recognizer (see Decisions).

Existing conventions this design follows (see `internal/ingest/sources/AGENTS.md`,
`internal/ingest/atsboard/AGENTS.md`, `internal/ingest/applyform/AGENTS.md`):
`Job` struct and `HydratingSource` interface (`internal/ingest/sources/source.go`), the
board-catalog `(provider, board)` model, and the `apply_forms` queue-drain fetcher registry.

## Goals / Non-Goals

**Goals:**

- Crawl any Dover-hosted company board (one board = one company slug) into the catalogue with full
  posting detail, following the existing hydrating-adapter shape.
- Recognize Dover board/job URLs for board-contribution intake without misfiring on the rest of
  `app.dover.com`'s shared host.
- Capture Dover application forms through the existing `apply_forms` queue-drain path.

**Non-Goals:**

- Discovering Dover companies automatically (no public directory/search exists on Dover, same as
  greenhouse/ashby/lever — boards are added one at a time via `board_submissions`/`cmd/add-board`).
- Submitting applications through Dover (Cloudflare Turnstile gates that flow; out of scope, and
  unrelated to `auto-apply`'s existing ATS coverage).
- Modeling equity compensation or visa sponsorship as first-class `Job` fields — freehire's schema
  has none today, and neither is common enough across other adapters to justify adding one now.

## Decisions

**Adapter shape: `HydratingSource`, modeled on `getro.go`, not `greenhouse.go`/`ashby.go`.**
Greenhouse and Ashby's list endpoints already carry the full description, so they implement plain
`Source.Fetch`. Dover's list endpoint (`careers-page/{client_id}/jobs`) does not — description,
compensation, and workplace detail only exist on the per-job `inbound/application-portal-job/{id}`
call. `getro.go` already has this exact two-step shape (`FetchNew(ctx, e, seen)`: list, then
`fetchDetails` over the unseen ids with a bounded worker pool, marking already-seen postings
`SeenRefresh = true` instead of re-fetching). `dover.go` follows the same structure rather than
inventing a second hydration pattern.

**A failed detail fetch falls back to a list-only `Job`, matching `getro.go`, not a drop.** The
Dover listing already carries `ExternalID`, `Title`, `WorkMode`, and location — enough for a usable
list-only posting, the same as getro's listing carries id/title/url/company without a description.
`getro.detail`'s failure path logs and returns the list-only `base` job rather than dropping the
posting; `dover.go` does the same. This also composes correctly with the pipeline's own
`HYDRATION_RETRY_DAYS` mechanism (`internal/ingest/AGENTS.md`), which withholds a stored,
description-less row from the seen-set so a later crawl re-attempts its detail fetch — dropping the
posting instead would forgo that retry path entirely and leave it invisible until the listing
merely repeats it as "unseen" again next run, which is strictly worse than storing what the listing
already gave.

**`Job.Company` comes from the configured board, not Dover's own `client_name`.** Every other
single-employer boarded adapter (`greenhouse.go:47`, `lever.go:90`, `workable.go:53`) sets
`Job.Company = e.Company`, the curator-entered name on the board catalog entry, not a platform-
reported field — company identity is deliberately owned by the catalogue, not by whatever string
each ATS happens to expose. `dover.go` follows the same rule; `client_name`/`client_domain` are
read only to build the constructed apply URL's slug sanity check (or not read at all — the slug is
already known from `e.Board`), never written into `Job.Company`.

**Board id = Dover company slug; client UUID resolved per crawl, not cached.**
The slug (`qompyl`) is the only stable, human-assignable board id available (it is what a curator
reads off the company's own career-page URL); the client UUID is an internal Dover id with no
public discovery path other than resolving it from the slug. Resolving it fresh each crawl is one
cheap extra call and avoids a second piece of stored state (a slug→UUID cache) that would need its
own invalidation story if Dover ever rotated a client's id.

**`atsboard` recognizer: allow-list the path's first segment, not a deny-list.**
Every existing `modePath` recognizer (e.g. `jobs.gusto.com`) assumes the whole host is dedicated to
job boards, so `reservedSegments` denies a short list of non-tenant paths (`boards`, etc.) and
treats everything else as a tenant slug. `app.dover.com` is the opposite shape: it is Dover's whole
product SPA (dashboard, `/crm`, `/docs/api`, sign-in, marketing), and job boards live under exactly
two known first segments, `apply` and `jobs`. Reusing the deny-list `modePath` mode here would mint
a false board out of `/pricing` or `/login`. This change adds a small allow-list check instead:
recognize only when the first path segment is exactly `apply` (`/apply/<slug>/<jobId>`) or `jobs`
(`/jobs/<slug>`), and decline every other path outright. This is new logic in `Recognize`, not a
reuse of `modePath`/`reservedSegments`.

**Application-form capture: register a `doverFetcher` in the existing `fetcherFor` registry, not
`Job.ApplyForm`.** `Job.ApplyForm` is documented as set only by an adapter whose *list* endpoint
already carries the form at zero extra cost (today, only `recruitee.go`). Dover's
`application_questions[]` lives on the per-job detail call, which the source adapter already makes
during hydration — but `Job.ApplyForm` would only ever be populated for newly-hydrated postings,
never for `SeenRefresh` ones, silently diverging from every other adapter's forms. The existing,
designed-for-this extension point is the queue-drain path: `capture-apply-form` already fetches a
queued posting's form on demand from the ATS's own API for greenhouse/ashby/workable/lever via
`fetcherFor`. Adding a `doverFetcher` (hitting the same `inbound/application-portal-job/{id}`
endpoint, mapping `application_questions[]` to `applyform.Field` the way `FromAshby` maps Ashby's
own question shape) fits this path exactly and needs no change to when or how forms get queued.

**No special HTTP transport.** The registry's special-cased fingerprint/stealth transport block
(used for a handful of adapters that need to look like a browser) is unnecessary here — verified
live that Dover's JSON API answers a bare `curl -A curl/8.0` with 200, no challenge. `dover.go`
registers as a plain one-liner in `registry.go`'s `All(c)`, using the ordinary `JSONGetter`.

**Facts without a structured field, and salary gating: reuse the `joppy-source` pattern.**
Equity terms (`equity_lower_bound/upper_bound`, `offers_equity`) and `visa_support` have no home in
`Job`; per the spec, they fold into the description text. `SalaryMin/Max/Currency` are set only
when `compensation.open_to_sharing_comp` is true, mirroring Joppy's "public salary only when the
employer opted in" requirement — Dover's own field name states the same opt-in semantics directly.

## Risks / Trade-offs

- **[Risk]** Dover could add bot protection to the JSON API later without notice, since today's
  openness is observed behavior, not a documented contract. → **Mitigation**: none needed at build
  time; a broken board surfaces through the existing board-health/chronic-board machinery
  (`close-chronic-boards`) like any other adapter regressing, no special-casing required now.
- **[Risk]** Only one live sample (`qompyl`, `workplace_type=REMOTE`, `employment_type=PART_TIME`,
  `open_to_sharing_comp=false`) has been observed; other enum spellings (`HYBRID`, `ONSITE`,
  `FULL_TIME`, `CONTRACT`, etc.) and a real opted-in salary range are unconfirmed. → **Mitigation**:
  `workplaceTypeMode`/the `EmploymentType` mapping already degrade to an empty/unmapped value on an
  unrecognized enum, matching every other adapter's convention — an unseen spelling loses a facet,
  never crashes or misclassifies.
- **[Trade-off]** Resolving the client UUID on every crawl (rather than caching it) costs one extra
  request per board per run. Accepted: it is one request against a board-scoped crawl that already
  makes at least two, and avoids a second cached-state invalidation path.

## Open Questions

- Confirm the `compensation.open_to_sharing_comp` gating field against a live Dover board that
  actually discloses a salary range (today's only sample has both false and null bounds, so the
  gate is inferred from the field's name, not observed toggling a real range). Does not change the
  spec or approach — only confirms the field empirically once such a board is added.
- Confirm `workplace_type`/`employment_type` enum spellings beyond `REMOTE`/`PART_TIME` once a
  second live Dover board is available for comparison. Same non-blocking note.
