## Why

A CV leaves the product and goes dark: between sending it and either a reply or silence, the
candidate learns nothing. A traced link answers one narrow question — was this CV opened at
all — which separates "they read it and passed" from "it never reached a human".

The evidence is indirect and inflated by mail-security scanners, so the feature is opt-in per
CV, off by default, and its surfaces report clicks as evidence rather than proof.

## What Changes

- A CV carries a `tracer_links_enabled` toggle, **off by default**, settable only by the
  candidate over the cookie-authenticated `PUT /me/cvs/:id`. It is deliberately absent from
  `PatchOps`, so the tailoring agent cannot enable tracking on the candidate's behalf.
- When the toggle is on, `GET /me/cvs/:id/pdf` mints a stable token per (CV, document
  position, destination) and renders each outbound link with the traced href while keeping the
  candidate's own link text visible.
- A new public endpoint `GET /cv/:token` records the click and 302s to the real destination.
  It is token-only: no URL is ever read from a query parameter, so it cannot serve as an open
  redirect.
- Clicks record time, a bot flag, device/OS/UA family, referrer host, and a salted
  `visitor_hash` (HMAC over IP + user agent) enabling a distinct-visitor count. No raw IP and
  no bare IP hash is stored. Clicks older than 180 days are deleted by `cmd/prune`.
- All six Typst templates emit clickable links. Five of them currently print links as plain
  text; with the visible text staying native, an unclickable link is untrackable, so this is a
  precondition of the feature rather than adjacent cleanup.
- The CV editor gains a per-link panel (clicks, distinct visitors, last click, bot filter).
  The tracking board's application card gains a "CV opened" marker.
- The privacy policy gains a paragraph covering what a traced link records, its opt-in nature,
  and the retention window.

## Capabilities

### New Capabilities
- `cv-tracer-links`: per-CV opt-in link tracing — the toggle and who may set it, token minting
  and its idempotency, the redirect endpoint and its click record, bot classification,
  visitor hashing and retention, and the two read surfaces.

### Modified Capabilities
- `cv-builder`: the on-demand PDF render consults the CV's tracer toggle and may substitute
  link hrefs while preserving visible text; every registered template must emit clickable
  links.
- `user-job-tracking`: the interaction listing carries the CV-opened timestamp for the
  application, so the board can show it beside the existing state.

## Impact

**Schema** — `cvs` gains `tracer_links_enabled` and `last_click_at`; new tables
`cv_tracer_links` and `cv_link_clicks`. `ALTER TABLE cvs` takes ACCESS EXCLUSIVE, so the
migration must not run in the 03–07 UTC `pg_dump` window.

**Code** — new `internal/tracerlink` (Fiber-free, pgx-free); `internal/handler/cv.go` (toggle,
mint-on-render, panel endpoint) and a public redirect route registered beside `/health`;
`internal/cv/renderer.go` render payload; all six `internal/cv/templates/*.typ`;
`internal/db/queries/`; `cmd/prune` for retention; `web/` CV editor and tracking board.

**Infrastructure** — `web/nginx.conf` must route `/cv/` to the backend, which today receives
only `/api/` and `/health`. New env var `TRACER_LINK_SALT`; without it the toggle cannot be
enabled.

**Deliberately unchanged** — the application silence clock. `last_click_at` must not enter
`last_activity_at`, for the same reason `followed_up_at` is kept out of it: someone opening a
CV is not a reply, and folding it in would clear the silence badge at the moment it matters
most.

**Reputation risk** — a redirect inside a PDF is a phishing pattern and mail gateways may flag
the domain. job-ops, the prior art here, pushes this onto whoever runs the self-hosted
instance; our operator is us, and `freehire.me` carries all other traffic.
