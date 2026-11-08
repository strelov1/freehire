## Context

`web/` is a client-only Vite + Svelte 5 SPA: `main.ts` mounts `App.svelte`,
which dispatches on a hand-rolled History-API router (`router.svelte.ts`,
routes: home, jobs, job/:slug, companies, company/:slug, my/jobs, my/api-keys).
Data is fetched in the browser via `lib/api.ts` against same-origin `/api`
(nginx proxies `/api`→Go in prod; the Vite dev proxy does the same). Auth is an
`HttpOnly; SameSite=Lax` cookie set by the API; the SPA can't read it and starts
signed-out, then `initAuth()` calls `/me` to hydrate user state. Theme is applied
pre-mount from `localStorage` to avoid a flash.

The production frontend is a static build served by nginx (`web/Dockerfile`,
`web/nginx.conf`); deploy runs from `../freehire-ops` (`deploy.sh` builds the
working-tree checkout). Because the SPA returns an empty shell on every URL,
crawlers and link-preview bots see no content (see proposal).

`web/` has **no JS test runner** (no vitest); frontend is verified via
`svelte-check` + lint + manual/e2e. `npm run lint` is red on `main` (oxlint masks
eslint) — use `svelte-check` as the green gate.

## Goals / Non-Goals

**Goals:**

- Server-render the public read pages (`/`, `/jobs/:slug`, `/companies`,
  `/companies/:slug`) so full content is in the initial HTML.
- Per-route `<title>`/`<meta>`/canonical/OG + `JobPosting`/`Organization` JSON-LD.
- Real `/robots.txt` and a generated `/sitemap.xml`.
- Preserve every existing route, behavior, and the same-origin auth-cookie flow.
- Keep the same dev ergonomics (one origin, `/api` proxied) and prod same-origin.

**Non-Goals:**

- No backend API changes beyond, if needed, a lightweight slug+timestamp listing
  for the sitemap. No product/UX redesign. No change to enrichment/ingest.
- No SEO content strategy (copywriting, keyword targeting) — only the technical
  render + structured-data substrate.
- No move to per-route prefetching/optimizations beyond what the migration needs.

## Decisions

### D1 — SvelteKit + `adapter-node`, SSR with hydration

Migrate to SvelteKit and ship `adapter-node` (the frontend becomes a long-lived
Node server). *Why:* canonical SSR with per-route `load` and `<svelte:head>`,
one source of markup, progressive hydration; matches the chosen approach.
*Alternatives rejected:* `adapter-static` + prerender (job pages are dynamic and
unbounded — can't fully prerender a live board); a Go-side SSR endpoint (would
duplicate the job render in Go and Svelte).

### D2 — Same-origin topology: nginx fronts both Node and the Go API

Keep nginx as the single public origin: `/api` (and `/health`) → Go `app:8080`;
everything else → the SvelteKit Node server. *Why:* preserves the `SameSite=Lax`
same-origin contract with zero auth changes, and isolates the migration to the
"static files" half of the current nginx config. Server-side `load` calls the Go
API over the **internal** service URL (server-to-server, no cookie-origin
issues); browser hydration keeps calling same-origin `/api`.
*Alternative rejected:* make the Node server the sole origin and have it proxy
`/api` — more moving parts in the web app for no cookie benefit.

### D3 — Data loading via `load` + `event.fetch` cookie forwarding

Public pages load data in `+page.server.ts` (or universal `+page.ts` where no
secrets/cookies are needed) using SvelteKit's `event.fetch`, which forwards the
incoming request's cookies on same-origin calls — so an authenticated SSR
request still carries the session. `lib/api.ts` is refactored to accept an
injected `fetch` so the same client works on server and client.
*Why:* one API client, correct cookie behavior, no duplicate fetch logic.

### D4 — Auth & theme without FOUC under SSR

Auth: a root `+layout.server.ts` resolves the current user from `/me` (cookie
forwarded) so the server renders the correct signed-in/out chrome; the user is
exposed via `page.data.user` (NOT a module-level `$state` singleton, which would
leak one request's user into another's SSR on a long-lived Node server), and the
client hydrates from that data instead of a post-mount `/me` flicker.
`login`/`logout` mutate via the API then `invalidateAll()` to re-resolve.
Theme: **revised from the cookie approach to a no-FOUC inline script in
`app.html`** that applies the `.dark` class from localStorage (or the OS for
`system`) before first paint. *Why the change:* the inline script handles
`system` mode correctly (the server can't know the OS preference), avoids a theme
module singleton on the server, and needs no cookie or `handle` hook — simpler
and still flash-free. `theme.svelte.ts` is made SSR-safe (every browser API
guarded by `browser`); the toggle defers its stateful icon to `onMount` to avoid
a hydration mismatch. *Why (auth):* SSR must emit correct chrome in the first
byte or we trade one flash for another.
*Alternative considered:* keep auth fully client-side (render signed-out, hydrate)
— simpler but reintroduces a visible auth flash on every load.

### D5 — Routing & URL-synced filters → SvelteKit primitives

Map routes 1:1 to `src/routes/**` (`+page.svelte`). Replace `router.navigate`
with `<a>`/`goto`, and the filter URL-sync (`setQuery`/`replaceState`,
`query`/`search`) with `$page.url.searchParams` + `replaceState`/`goto({replaceState})`.
Preserve the **synchronous write-state→URL in the input handler** pattern (a
prior bug: a separate `$effect` made the controlled input revert each keystroke).
*Why:* SvelteKit owns navigation; the hand-rolled router is retired, not wrapped.

### D6 — SEO artifacts

- `JobPosting` JSON-LD built from the job-view shape on `/jobs/:slug`;
  `Organization` on company pages. Emitted server-side in `<svelte:head>`.
- `/robots.txt` as a static route (allow all, point at the sitemap).
- `/sitemap.xml` as a server route (`+server.ts`) enumerating job and company
  pages. *Source:* prefer an existing public listing; if pagination over the full
  jobs list is too heavy, add a minimal `slug + updated_at` listing query
  (flagged as the one allowed backend touch).

## Risks / Trade-offs

- **Deploy topology change (static nginx → Node server).** → Stage the new
  `web/Dockerfile` (Node runtime) and nginx config together; verify locally via
  Docker before touching `../freehire-ops`; keep rollback = redeploy the previous
  static image. Coordinate the ops change as its own step.
- **Hydration mismatches** (server vs client markup, esp. theme/auth/dates). →
  Make theme/auth server-resolved (D4); render dates deterministically; rely on
  `svelte-check` + manual hydration check (no console warnings) as the gate.
- **Auth cookie not forwarded on SSR fetch** → use `event.fetch` (not bare
  `fetch`) and the internal API URL; explicitly test a signed-in SSR load.
- **No JS test runner** → behavior is locked by `svelte-check`, lint (against the
  known-red baseline), and a manual verification pass per OpenSpec scenario;
  don't add vitest just for this change.
- **Sitemap cost on a large board** → cap/segment if needed and `log` what's
  omitted rather than silently truncating.
- **SEO regressions from wrong canonical/duplicate URLs** → one canonical host,
  canonical tag per page, trailing-slash policy fixed once in config.

## Migration Plan

1. Scaffold SvelteKit in `web/` (config, `app.html`, `src/routes/`), `adapter-node`;
   keep `lib/` components/`api.ts`/`types.ts`/stores.
2. Port routes view-by-view to `+page.svelte` (+ `load`), retiring `App.svelte`
   and `router.svelte.ts`; preserve filter URL-sync and auth/theme (D3–D5).
3. Add SEO: per-route head, JSON-LD, `robots.txt`, `sitemap.xml` (D6).
4. New `web/Dockerfile` (Node runtime) + nginx config (D2); verify in Docker.
5. Roll out via `../freehire-ops` as a distinct deploy step; rollback = previous
   static image.

## Open Questions

- Sitemap source: reuse `/api/v1/jobs` pagination, or add a minimal
  slug+timestamp listing query? (Decide during the sitemap task based on volume.)
- Does the root layout need `/me` on **every** SSR request (cost), or only where
  chrome depends on it? Start with layout-level; revisit if it adds latency.
