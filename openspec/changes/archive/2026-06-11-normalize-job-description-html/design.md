## Context

Three source adapters (`internal/sources/{greenhouse,lever,ashby}.go`) each populate `Job.Description` differently, and the SPA renders `job.description` as escaped plain text (`JobView.svelte`). Evidence gathered during debugging:

- greenhouse `content` is **entity-encoded HTML** (`&lt;h2&gt;…`, 20550 chars for one Dropbox role) → renders as visible entities.
- lever exposes the body across `description`, `lists[]` (heading `text` + HTML `content`), and `additional`; its `*Plain` mirrors are unreliable (empty for the reported Spotify role) → adapter stores 0 chars.
- ashby exposes both `descriptionPlain` (used today) and `descriptionHtml`.

The decision (made with the user) is to make `description` **sanitized formatted HTML** and render it with Svelte `{@html}`.

## Goals / Non-Goals

**Goals:**

- Every adapter yields description as sanitized HTML from the platform's authoritative HTML field(s).
- Server-side sanitization closes the `{@html}` XSS surface; unsanitized markup is never persisted.
- SPA renders descriptions as formatted HTML with readable prose styling.
- Existing rows backfilled via idempotent re-ingest.

**Non-Goals:**

- No schema/migration change (`description` column already exists).
- No change to the jobview wire shape (still a `description` string).
- Not touching enrichment, search indexing semantics, or other adapters' fields beyond description.
- Not building a generic HTML-to-Markdown or rich-text editor pipeline.

## Decisions

**1. Output format = sanitized HTML (not plain text).**
Chosen over stripping to plain text because job descriptions are inherently structured (headings, bullet lists, emphasis) and the user wants that preserved. Trade-off: introduces an XSS surface, mitigated by mandatory server-side sanitization.

**2. Sanitize server-side at ingest, store clean HTML.** Alternatives: (a) sanitize at read time in the handler, (b) sanitize client-side. We store sanitized HTML so the database is the trust boundary — every reader (API, search index, future consumers) gets safe content for free, and sanitization cost is paid once per ingest, not per request.

**3. Sanitizer = `github.com/microcosm-cc/bluemonday` `UGCPolicy`.** It is the de-facto standard Go HTML sanitizer, well-maintained, and `UGCPolicy` allows exactly the structural tags we want (headings, p, ul/ol/li, strong/em, a) while stripping scripts, styles, and event handlers. Alternative (`x/net/html` hand-rolled allowlist) rejected: reinventing a security-sensitive component is the kind of "clever shim" we avoid.

**4. A single shared `sanitizeHTML` helper in `internal/sources`.** All three adapters call it on their assembled HTML. The bluemonday policy is compiled once (package-level), since policies are safe for concurrent reuse. Keeps the security decision in one place rather than scattered per adapter.

**5. Per-adapter HTML acquisition stays in each adapter** (greenhouse decode, lever assemble, ashby field switch), because the multi-field structure is platform-specific and the pipeline does not see it. Only the final `sanitizeHTML` step is shared.

**6. SPA rendering.** `JobView.svelte` switches `{job.description}` → `{@html job.description}` inside a styled prose container (e.g. Tailwind `prose`-like classes or scoped element styles) so headings/lists render legibly. Because content is already server-sanitized, `{@html}` is safe here.

## Risks / Trade-offs

- **XSS via `{@html}`** → Mitigated by mandatory server-side bluemonday sanitization before persistence; a test asserts `<script>`/`onclick` are stripped.
- **Lever assembly producing awkward output for postings without lists** → Assembly conditionally includes only non-empty sections; covered by tests for both a multi-list posting and a description-only posting.
- **Greenhouse double-encoding edge cases** (`&amp;amp;`) → `html.UnescapeString` handles one decode level; bluemonday re-escapes stray text. Acceptable; the common single-encoded case is covered by a test.
- **Stale rows until re-ingest** → Re-ingest is part of the change's verification step; upsert idempotency means it is safe to re-run.
- **bluemonday strips an unexpectedly useful tag** → `UGCPolicy` is permissive for prose; if a needed tag is missing we extend the shared policy in one place.

## Migration Plan

1. Land adapter + sanitizer changes with tests (TDD).
2. Update SPA render.
3. Re-ingest all boards (`go run ./cmd/ingest`) to backfill descriptions — idempotent upsert, no manual data migration.
4. Rollback: revert the commit; descriptions revert to prior (broken) values on next ingest. No schema to undo.

## Open Questions

- None blocking. Prose styling specifics in the SPA are a presentation detail to settle during implementation.
