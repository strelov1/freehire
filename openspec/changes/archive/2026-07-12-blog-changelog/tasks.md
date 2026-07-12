## 1. Tooling & dependencies

- [x] 1.1 Add `mdsvex` to `web/package.json` (dev dep) and install; register the mdsvex preprocessor + `.svx` extension in `web/svelte.config.js` alongside `vitePreprocess`, leaving `.svelte` and CSP intact
- [x] 1.2 Verify `npm run build` and existing `svelte-check` still pass with the preprocessor wired (no route changes yet)

## 2. Content model & loader

- [x] 2.1 Create the content directory `web/src/posts/` with one seed `changelog` post (`*.svx`) carrying full frontmatter (`title`, `date`, `summary`, `type`, `tags`, `draft`)
- [x] 2.2 Implement the loader: pure core in `web/src/lib/blog.ts` (typed `PostMeta` incl. `type` enum default `changelog`, `slugFromPath`, `parseFrontmatter` with required-field + `type`-enum validation throwing the offending file name, newest-first `selectPosts`) + `web/src/lib/blogPosts.ts` (`import.meta.glob` discovery, `draft` gate on `import.meta.env.DEV`, `listPosts()` / `getPost(slug)`) — split so the pure core runs under the plain-Node vitest config
- [x] 2.3 Unit-test the loader's pure logic (sort order, draft filtering, missing-field + bad-`type` validation, default type) in `web/src/lib/blog.test.ts`

## 3. Blog pages

- [x] 3.1 `web/src/routes/blog/+page.ts` (loads `listPosts()`) + `+page.svelte` rendering the newest-first list (title link, date, summary, type badge, tags) with a client-side All / Changelog / Articles filter, styled to match existing content pages
- [x] 3.2 `web/src/routes/blog/[slug]/+page.ts` loading `getPost(params.slug)` with `error(404)` on miss or prod-draft, + `+page.svelte` rendering the compiled body via `<svelte:component>` with title/date

## 4. SEO

- [x] 4.1 Post page `<svelte:head>`: `<title>`, meta description, Open Graph (`article`) tags, and `Article` JSON-LD from the post metadata
- [x] 4.2 Add `blogPaths()` to `web/src/lib/sitemap.ts` and spread `/blog` + published post URLs into `sitemap-pages.xml`'s `GET`; cover `blogPaths()` in a sitemap unit test
- [x] 4.3 `$lib/server/og/blog.ts` `buildBlogCard(post)` + `web/src/routes/blog/[slug]/og.png/+server.ts` rendering a 1200×630 card via `renderMarkupPng`/`loadOgFonts`; unknown/draft slug → 404

## 5. RSS

- [x] 5.1 `web/src/routes/blog/rss.xml/+server.ts`: valid RSS 2.0 from `listPosts()` (title/link/guid/pubDate/description), newest-first, drafts excluded, XML-escaped; unit-test the feed builder

## 6. Authoring discipline (skill + workflow hooks)

- [x] 6.1 Create `.claude/skills/write-changelog/SKILL.md` (note: `.claude/` is gitignored — local tooling, not in the branch): invokable at feature completion — offers a `type: changelog` post (short, from the shipped diff/summary), then offers to draft a `type: article` post; writes `.svx` into `web/src/posts/` in the exact frontmatter contract from task 2.2
- [x] 6.2 Add a Finish-step to `.claude/skills/spec-driven-tdd/SKILL.md` (gitignored local tooling) (`### 4. Finish`) invoking the write-changelog skill after archive+sync
- [x] 6.3 Add a one-line rule to `AGENT.md` (via the `CLAUDE.md → AGENT.md` symlink) that a completed user-facing feature ends by offering a changelog entry, then a blog post

## 7. Verification

- [x] 7.1 Run `npm run build`, `svelte-check`, `vitest`, and `eslint` in `web/`; manually verify `/blog` (incl. type filter), a post page, `/blog/<slug>/og.png`, `/blog/rss.xml`, and the post's presence in `sitemap-pages.xml`
- [x] 7.2 Sanity-check the write-changelog skill end-to-end: invoke it, confirm it produces a valid `.svx` post the loader accepts (build stays green)
