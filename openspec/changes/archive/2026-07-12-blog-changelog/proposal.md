## Why

freehire ships product changes continuously but has no public place to announce them — new sources, features, and fixes are invisible to users and to search engines. A markdown-authored changelog blog gives the product a voice (release notes, product news), an SEO-indexable content surface consistent with the existing collections/companies landing pages, and an RSS feed users can subscribe to for updates.

## What Changes

- Add a `/blog` index page listing posts newest-first (title, date, summary, tags, type badge), with a lightweight type filter (All / Changelog / Articles).
- Add a `/blog/[slug]` post page rendering a markdown article with SSR.
- Author posts as markdown files committed in the repo (git owns the content, mirroring the `sources/*.yml` convention), compiled at build time via **mdsvex**.
- Each post carries typed frontmatter (title, date, summary, type, tags, draft) that drives listing order, filtering, metadata, and SEO. `type` is one of `changelog` (short release note) or `article` (long-form post) — one feed, two content tiers.
- Wire posts into SEO: per-post `<title>`/meta description, Open Graph tags, and `Article` JSON-LD; include published posts in `sitemap-pages.xml`.
- Add a `/blog/rss.xml` feed of published posts for subscription.
- Drafts (`draft: true`) are excluded from the index, sitemap, RSS, and (in production) direct access.
- Add an **authoring discipline** so shipped work reaches the changelog: an invokable skill that, on feature completion, offers to add a `changelog` post and then to draft a longer `article`; a Finish-step in the `spec-driven-tdd` skill and a rule in `AGENT.md` that trigger it.

Non-goals (deferred): a "latest updates" block on the landing page/footer, tag-filtered archive pages, author profiles, comments, and any DB/backend storage — the blog is fully static-content, frontend-only.

## Capabilities

### New Capabilities
- `blog-changelog`: a markdown-file-backed blog in the SvelteKit frontend — post authoring format (frontmatter incl. `type` + mdsvex), the `/blog` index (with type filter) and `/blog/[slug]` post pages, SEO metadata (meta/OG/JSON-LD + sitemap inclusion), and the RSS feed. The feature also ships an authoring-discipline skill + workflow hooks (Finish-step, AGENT.md) as tooling deliverables (no runtime spec requirement — they govern agent workflow, not system behavior).

### Modified Capabilities
<!-- None: the blog is additive. sitemap-pages.xml is extended, not respecified. -->

## Impact

- **Frontend only** (`web/`): new routes under `web/src/routes/blog/`, a new content directory for markdown posts, a small blog content-loader module in `web/src/lib/`, and an extension to the existing `sitemap-pages.xml` route.
- **Dependencies**: adds `mdsvex` (+ its remark/rehype needs) and a frontmatter parser to `web/package.json`; registers the mdsvex preprocessor and `.svx`/`.md` extension in `web/svelte.config.js`.
- **Tooling/workflow**: a new project skill under `.claude/skills/` (changelog/blog authoring), a Finish-step addition in `.claude/skills/spec-driven-tdd/SKILL.md`, and a rule in `AGENT.md` (edited via the `CLAUDE.md → AGENT.md` symlink).
- **Backend**: none. No new API, DB table, migration, or Go code.
- **Ops**: none beyond the standard web build/deploy; no new env vars.
