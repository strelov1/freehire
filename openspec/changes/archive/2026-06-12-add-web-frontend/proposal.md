## Why

The `hire` backend exposes a read-only HTTP API for jobs and companies but has
no user-facing client. A minimal web frontend makes the aggregated data
browsable and gives the project a usable surface on top of the existing API.

## What Changes

- Add a Svelte 5 + Vite + Tailwind v4 single-page app under `web/`.
- Screens: jobs list (paginated), job detail, companies list (with job count),
  company detail (company + its jobs) — covering the full current API.
- Flat light/dark/system theme persisted in localStorage.
- Enable cross-origin browser access on the API by adding CORS middleware to the
  Fiber server (the only backend change).

## Capabilities

### New Capabilities

- `web-frontend`: A browser SPA that lists and shows jobs and companies over the
  existing read-only API, including the API's CORS support that enables direct
  browser access.

### Modified Capabilities
<!-- None. The companies/jobs read endpoints are consumed as-is; no requirement
     changes to existing specs. -->

## Impact

- New `web/` directory: Svelte 5, Vite 6, TypeScript, Tailwind v4,
  shadcn-svelte (bits-ui), tailwind-variants, @lucide/svelte.
- Backend: add `github.com/gofiber/fiber/v2/middleware/cors` and wire it in
  `internal/handler/handler.go` (`Register`); new Go dependency in `go.mod`.
- No changes to the database, existing handlers, or response shapes.
