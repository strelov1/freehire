## Why

Google's own careers site (`www.google.com/about/careers/applications/jobs/results`) lists
~3,640 open Google roles, but the old public `careers.google.com/api/v3/search` endpoint is
gone (404). An earlier feasibility pass therefore marked Google "blocked." A fresh probe
shows the new site is **server-rendered**: every page embeds its full job payload inline in
an `AF_initDataCallback({key:'ds:1', data:[…]})` script block — no headless browser, no
auth, no GraphQL persisted-query. That makes Google a clean, high-value single-company
source to fold into the shared pool, alongside the existing `uber`/`amazon` boardless
adapters.

## What Changes

- Add a `google` source adapter (`internal/sources/google.go`) speaking the existing
  `Source` interface, registered with one `NewGoogle(c)` line in `sources.All`.
- It is a **boardless single-company** adapter (like `uber`/`amazon`): one company = all of
  Google, no per-tenant board id.
- The list page is paged via `?page=N` (20 jobs/page). The adapter fetches each page's HTML,
  extracts the `ds:1` `AF_initDataCallback` JSON blob, and reads the embedded job records —
  **no per-job detail fetch** (the list payload already carries title, description,
  responsibilities, qualifications, locations, and dates). It stops when a page yields no
  records or the running count reaches the payload's total-count field.
- Each posting maps to the normalized job shape: `external_id` = Google's numeric job id;
  `url` = the public `…/jobs/results/<id>` page, id only (NOT the sign-in-gated apply link);
  `description` = sanitized HTML assembled from the description + responsibilities +
  qualifications fields; `location` from the structured locations array; `posted_at` from the
  embedded Unix-epoch timestamp.
- Add one `google` entry to a `sources/*.yml` board file.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline and write path unchanged. -->

### Modified Capabilities
- `source-ingest`: add a requirement that `google` is a registered boardless provider — a
  server-rendered list adapter that extracts the embedded `ds:1` JSON payload and yields the
  normalized job shape with a sanitized-HTML description, paging until the catalogue is
  exhausted.

## Impact

- **New code**: `internal/sources/google.go` + `internal/sources/google_test.go`; one
  registration line in `internal/sources/source.go` (`sources.All`).
- **Config**: one new `sources/*.yml` entry. No new env vars.
- **DB**: none — reuses `UpsertJob` (`source = "google"`, namespaced `external_id`). No
  migration.
- **Dependencies**: none — uses the existing shared `HTTPClient.GetHTML`, the `html.go` DOM
  walk, and `sanitizeHTML`.
- **Deploy**: the adapter compiles into the existing ingest binary, so it needs an image
  rebuild + redeploy (not just a sources rsync) to run in prod, and a cron schedule for the
  new board file.
- **Out of scope (known seams)**: the positional `ds:1` array is brittle — if Google
  reorders fields the mapping breaks; a fixture-backed test guards the current layout. The
  apply link is sign-in-gated, so the canonical results page is used as `url`.
