## Context

`web/` is a SvelteKit app served SSR via `adapter-node`, fronted by nginx. It has
no markdown tooling today — content pages (`/about`, `/docs/api`, `/privacy`) are
hand-written Svelte. The site invests heavily in SEO: a chunked sitemap
(`$lib/sitemap`), dynamically rendered OG cards (`$lib/server/og` →
`renderMarkupPng` + `buildCompanyCard`), and JSON-LD on entity pages. The blog
must fit these existing seams rather than introduce a parallel content system.

Content is git-owned (mirroring the `sources/*.yml` convention): posts are
markdown files in the repo, compiled at build time. No backend, DB, or API is
involved.

## Goals / Non-Goals

**Goals:**

- Author changelog posts as markdown files with typed frontmatter.
- `/blog` index (newest-first) and `/blog/[slug]` post pages, SSR.
- SEO parity with the rest of the site: meta/OG/JSON-LD + sitemap inclusion.
- `/blog/rss.xml` feed.
- Drafts excluded from index, sitemap, RSS, and (in prod) direct access.

**Non-Goals:**

- Landing-page/footer "latest updates" block (deferred).
- Tag-filtered archive pages, author profiles, comments, pagination.
- Any DB/backend storage or API. Fully static, frontend-only.

## Decisions

### D1 — mdsvex for build-time compilation

Author posts as `.svx` (mdsvex) files under `web/src/posts/`. Register the mdsvex
preprocessor in `web/svelte.config.js` with `.svx` added to `extensions`, so
posts compile to Svelte components at build time. This gives SSR HTML with zero
runtime markdown parsing and allows richer content later (embedded components,
code highlighting) without changing the content model.

*Alternative considered:* runtime `marked` + a frontmatter parser read via
`import.meta.glob('...', { as: 'raw' })` and parsed in `+page.server.ts`. Simpler
dependency-wise but parses on every request, offers no component embedding, and
needs its own HTML-sanitisation story. mdsvex is the SvelteKit-idiomatic choice
and the user selected it.

### D2 — Content discovery via `import.meta.glob`

A single loader module `web/src/lib/blog.ts` uses
`import.meta.glob('/src/posts/*.svx', { eager: true })` to collect every post's
compiled component **and** its exported `metadata` (mdsvex exposes frontmatter as
a named `metadata` export). The loader:

- derives `slug` from the file basename,
- validates required frontmatter fields (throws at module-eval / build time on a
  missing `title`/`date`/`summary` — satisfies the "build fails" scenario),
- filters `draft` posts **except** in dev (so authors can preview drafts locally;
  `import.meta.env.DEV` gate),
- sorts by `date` descending,
- exposes `listPosts()` (metadata only, for index/sitemap/rss) and
  `getPost(slug)` (component + metadata, for the post page).

Keeping discovery in one typed module means the index, post page, sitemap, and
RSS all read one source of truth.

### D3 — Routes

- `web/src/routes/blog/+page.ts` → `listPosts()`; `+page.svelte` renders the list.
- `web/src/routes/blog/[slug]/+page.ts` → `getPost(params.slug)`, `error(404)` on
  miss or (in prod) draft; `+page.svelte` renders `<svelte:component>` for the
  body + `<svelte:head>` meta/OG/JSON-LD.
- `web/src/routes/blog/[slug]/og.png/+server.ts` → per-post OG card.
- `web/src/routes/blog/rss.xml/+server.ts` → RSS 2.0.

Loaders are `+page.ts` (universal) because the content is static and bundled — no
server-only secret is needed, and mdsvex components must resolve on both sides.

### D4 — SEO wiring

- **Sitemap:** extend `web/src/lib/sitemap.ts` with a `blogPaths()` helper
  (`/blog` + published slugs from `listPosts()`), spread into
  `sitemap-pages.xml`'s `GET` alongside `STATIC_PATHS`/`collectionPaths()`.
- **OG image:** add `$lib/server/og/blog.ts` `buildBlogCard(post)` mirroring
  `buildCompanyCard`, rendered by the existing `renderMarkupPng` + `loadOgFonts`.
  The route resolves the post via `listPosts()`; unknown/draft → 404.
- **JSON-LD / meta:** emitted in the post `+page.svelte` `<svelte:head>` from the
  loaded metadata (same pattern as company/job detail pages).

### D5 — RSS

Hand-build RSS 2.0 in the `rss.xml` route using the same XML-escaping discipline
as `$lib/sitemap` (reuse/mirror `escapeXml`). Items map from `listPosts()`;
`pubDate` from `date`, `link`/`guid` from the absolute post URL, `description`
from `summary`. No new dependency.

### D6 — One feed, two types

Posts carry a `type` of `changelog` (short release note, the default) or
`article` (long-form). It is one content collection, one loader, one `/blog`
feed — `type` is just a frontmatter enum the loader validates and exposes. The
index renders a per-post type badge and a lightweight **client-side** filter
(All / Changelog / Articles tabs over the already-loaded list — no extra route,
no server round-trip). Sitemap, RSS, and OG treat both types identically. This
keeps the "two tiers" the user wants without a second engine.

### D7 — Authoring discipline (skill + workflow hooks)

Shipping the surface isn't enough — the habit of writing to it is the other half.
Three tooling deliverables, no runtime behaviour:

- **Skill** `.claude/skills/write-changelog/SKILL.md`: invokable at feature
  completion. It offers to create a `type: changelog` post (short, from the
  shipped diff/summary), then offers to draft a `type: article` long-form post.
  It writes `.svx` files into `web/src/posts/` in the exact frontmatter format
  this change defines, so the skill and the loader share one contract.
- **Finish-step** in `.claude/skills/spec-driven-tdd/SKILL.md`: a new item in the
  `### 4. Finish` list — after archive+sync, invoke the write-changelog skill.
- **AGENT.md rule**: a one-line convention (general, not spec-driven-only) that
  completing a user-facing feature ends by offering a changelog entry, then a
  blog post. Edited via the `CLAUDE.md → AGENT.md` symlink (see project memory).

These are agent-workflow tooling, so they get **tasks** but no `spec.md`
requirement (specs describe runtime system behaviour, which this is not).

## Risks / Trade-offs

- **mdsvex preprocessor touches the global build** → a misconfigured
  `svelte.config.js` could break the whole web build. Mitigation: add mdsvex as an
  additional preprocessor alongside `vitePreprocess` and add `.svx` to
  `extensions` (leaving `.svelte` intact); verify `npm run build` + existing
  `svelte-check` still pass before merge.
- **CSP:** the site sets a strict `script-src`. mdsvex output is compiled into the
  same-origin bundle (not inline `<script>`), so no CSP change is needed. Risk if a
  post embeds a raw `<script>` — out of scope; posts are trusted git content.
- **Frontmatter validation throwing at build** could make a typo fail CI. That is
  the intended behaviour (fail loud, not ship blank metadata); the error names the
  file.
- **Draft-in-dev vs prod divergence** → a draft renders locally but 404s in prod.
  Mitigation: single `import.meta.env.DEV` gate in the loader so all four surfaces
  (index, post, sitemap, rss) agree.
- **Per-post OG rendering cost** → reuses the cached, on-demand pattern already
  proven for companies/jobs (hour cache + SWR); low incremental risk.

## Migration Plan

Additive, frontend-only. Deploy is the standard web build/deploy; no migration, no
env var, no backend change. Rollback = revert the branch. A seed post is included
so `/blog` is non-empty on first ship.
