## Why

The `jobs.description` column is populated inconsistently across source adapters, and the SPA renders it as escaped plain text. Two adapters violate the rendering contract, producing user-visible bugs:

- **greenhouse** stores HTML-entity-encoded HTML (e.g. `&lt;h2&gt;Role Description&lt;/h2&gt;`). The SPA escapes it again, so users see raw entities instead of text (e.g. Dropbox "Data Engineer", 20550 chars of garbage).
- **lever** stores only `descriptionPlain`, which Lever frequently leaves empty or partial because the body is split across `description` + `lists[]` + `additional`. Result: 0 characters stored (e.g. Spotify "Partner Marketing Manager"), i.e. "too little data".
- **ashby** stores `descriptionPlain` — correct today, but plain (loses structure).

There is no normalization step reconciling each platform's description format with what the frontend renders. We are fixing that root cause.

## What Changes

- Each adapter yields the job description as **HTML** sourced from the platform's authoritative HTML field(s):
  - greenhouse: `html.UnescapeString(content)` (its `content` is entity-encoded HTML).
  - lever: assemble `description` + per-list `<h3>{list.text}</h3>{list.content}` + `additional`.
  - ashby: use `descriptionHtml` instead of `descriptionPlain`.
- A shared **server-side HTML sanitizer** is applied before persisting, so the stored description is safe to render directly. Unsanitized source HTML is never stored.
- The SPA renders the description as formatted HTML (`{@html}`) with prose styling, replacing the escaped plain-text render.
- Existing rows are backfilled by re-running ingest (the upsert is idempotent on the dedup key).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `source-ingest`: the "Adapter maps a posting to the normalized job shape" requirement is strengthened — the description an adapter yields SHALL be sanitized HTML assembled from the platform's authoritative HTML fields, not raw or entity-encoded source content, and not a partial plain-text field.

## Impact

- **Code**: `internal/sources/{greenhouse,lever,ashby}.go` (description extraction), a new shared sanitizer in `internal/sources` (or `internal/normalize`), `web/src/lib/components/JobView.svelte` (render via `{@html}` + prose styles).
- **Dependencies**: adds a Go HTML sanitizer (`github.com/microcosm-cc/bluemonday`).
- **Data**: requires a re-ingest to backfill existing `jobs.description` values; no schema/migration change (column already exists).
- **Security**: `{@html}` introduces an XSS surface that the server-side sanitizer is responsible for closing — sanitization is a hard requirement, not optional.
