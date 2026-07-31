## 1. Templates emit clickable links

- [x] 1.1 Add a registry-driven test asserting every template in `internal/cv/template.go`
      renders header links and a project link as clickable links; it must fail today for the
      five templates that print them as inert text
- [x] 1.2 Make `portrait`, `sidebar`, `centered`, `modern-sans` and `headshot` emit `link()`,
      keeping each template's own type scale (every internal `size:` an em multiple of its base)
- [x] 1.3 Regenerate the gallery previews with `make cv-previews` and commit the changed SVGs
- [x] 1.4 Install the prod-pinned typst in CI, so the registry guard runs instead of skipping —
      a guard that skips is not a guard
- [x] 1.5 The requirement's "so that a reader can follow them" is only satisfied once 5.1b
      lands; group 1 is not shippable on its own

## 2. Domain package `internal/tracerlink`

- [x] 2.1 `Targets(doc)` — eligible links with their `source_path`: scheme-less URLs normalise
      to https, while `mailto:`, `tel:`, empty values and our own domain are skipped
- [x] 2.2 `Token(prefix)` — `<prefix>-<rrrrr>`, five random lowercase alphanumerics
- [x] 2.3 `Classify(method, userAgent)` — bot flag (pattern list plus "any non-GET is a bot"),
      device type, OS family, UA family
- [x] 2.4 `VisitorHash(salt, ip, userAgent)` — keyed HMAC, stable for one visitor and empty
      when no salt is configured

## 3. Schema and queries

- [x] 3.1 Migration `0060_cv_tracer_links.sql` — `cvs.tracer_links_enabled`,
      `cvs.last_click_at`, tables `cv_tracer_links` and `cv_link_clicks` with their indexes
- [x] 3.2 Queries in `internal/db/queries/tracer_links.sql`: idempotent `UpsertTracerLink`,
      `TracerLinkByToken`, `RecordTracerClick` (stamping `cvs.last_click_at` in the same
      transaction for countable clicks), `TracerLinkStatsForCV`, `DeleteExpiredTracerClicks`;
      run `make sqlc`
- [x] 3.3 Integration test (build tag): re-minting an unchanged CV reuses tokens; a changed
      destination mints a new one while the old still resolves; one destination at two
      positions gets two tokens

## 4. Toggle

- [x] 4.1 `PUT /me/cvs/:id/tracer-links` sets the flag, owner-scoped, refusing to enable it when
      no salt is configured — its own route, outside cvedit, so an undo cannot revoke consent
- [x] 4.2 Test that `PATCH /me/cvs/:id` cannot set it — the field is not in `PatchOps`

## 5. Traced rendering

- [x] 5.1 `renderPayload` carries `link_hrefs` for header links and projects; a CV with tracing
      off renders visually unchanged (compare SVG — a Typst PDF embeds a timestamp)
- [x] 5.1b The href is absolute even when tracing is off: links are stored scheme-less
      (`github.com/ada`), so today every template — `classic-ats` included, since long before
      this change — emits a relative URI that no PDF reader can follow. Normalise in the
      payload, not in six copies of Typst string handling
- [x] 5.2 `RenderCVPDF` mints tokens and passes the traced hrefs when the CV has tracing on,
      leaving `cvs.data` untouched
- [x] 5.3 Test that the extracted text layer still carries the candidate's own link text

## 6. Redirect endpoint

- [x] 6.1 `GET /cv/:token` beside `/health`: resolve, record, `302`; unknown or deleted token
      yields `410` with an explanatory page
- [x] 6.2 The destination comes only from the stored token — no query parameter, path remainder
      or header can influence it
- [x] 6.3 A failing click write still redirects
- [x] 6.4 A click carrying a valid session for the CV's owner is marked as the owner's own and
      excluded from counts and from `cvs.last_click_at`
- [x] 6.5 `location /cv/ { proxy_pass $backend; }` in `web/nginx.conf`

## 7. Read surfaces

- [x] 7.1 `GET /me/cvs/:id/tracer-links` — per link: destination, clicks, distinct visitors,
      last click; bots counted separately; owner-scoped; empty list for an untraced CV. The CV
      read reports the toggle's own state, and the toggle query gained its integration test
- [x] 7.2 `ListUserJobs` carries `cv_opened_at` for application rows, read from
      `cvs.last_click_at` via `cvs.job_id`
- [x] 7.3 Test that a recorded click leaves `last_activity_at`, `days_silent` and
      `silence_state` unchanged

## 8. Web

- [x] 8.1 CV editor: the toggle with its plain-language explanation of what gets recorded, and
      the per-link panel with the "include likely bots" switch
- [x] 8.2 Tracking board: the CV-opened marker beside the existing state, worded as evidence
      rather than proof
- [x] 8.3 Verify visually in a real browser — the 410 page shipped unstyled, reading as a
      server fault rather than an explanation; it is the only surface a stranger sees

## 9. Retention and configuration

- [x] 9.1 `cmd/prune` deletes click records older than 180 days, dry-run by default like every
      other removal it performs
- [x] 9.2 `TRACER_LINK_SALT` documented in the environment reference and `.env.example`

## 10. Documentation

- [x] 10.1 Privacy policy: what a traced link records, opt-in per CV and off by default, the
      180-day window, and that the owner's own clicks are not counted
- [x] 10.2 `internal/cv/AGENTS.md`: adding a template now requires emitting `link()`, and why
      the registry test exists
- [ ] 10.3 Offer a `/blog` changelog entry once the feature ships
