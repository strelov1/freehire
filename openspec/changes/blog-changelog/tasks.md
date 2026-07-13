## 1. Tooling & dependencies

- [ ] 1.1 Add `mdsvex` to `web/package.json` (dev dep) and install; register the mdsvex preprocessor + `.svx` extension in `web/svelte.config.js` alongside `vitePreprocess`, leaving `.svelte` and CSP intact
- [ ] 1.2 Verify `npm run build` and existing `svelte-check` still pass with the preprocessor wired (no route changes yet)

## 2. Content model & loader

- [ ] 2.1 Create the content directory `web/src/posts/` with one seed post (`*.svx`) carrying full frontmatter (`title`, `date`, `summary`, `tags`, `draft`)
- [ ] 2.2 Implement `web/src/lib/blog.ts`: typed `PostMeta`, `import.meta.glob` discovery, slug-from-filename, required-field validation (throws at build with the offending file name), `draft` filtering gated on `import.meta.env.DEV`, newest-first sort, and `listPosts()` / `getPost(slug)`
- [ ] 2.3 Unit-test the loader's pure logic (sort order, draft filtering, missing-field validation) in `web/src/lib/blog.test.ts`

## 3. Blog pages

- [ ] 3.1 `web/src/routes/blog/+page.ts` (loads `listPosts()`) + `+page.svelte` rendering the newest-first list (title link, date, summary, tags), styled to match existing content pages
- [ ] 3.2 `web/src/routes/blog/[slug]/+page.ts` loading `getPost(params.slug)` with `error(404)` on miss or prod-draft, + `+page.svelte` rendering the compiled body via `<svelte:component>` with title/date

## 4. SEO

- [ ] 4.1 Post page `<svelte:head>`: `<title>`, meta description, Open Graph (`article`) tags, and `Article` JSON-LD from the post metadata
- [ ] 4.2 Add `blogPaths()` to `web/src/lib/sitemap.ts` and spread `/blog` + published post URLs into `sitemap-pages.xml`'s `GET`; cover `blogPaths()` in a sitemap unit test
- [ ] 4.3 `$lib/server/og/blog.ts` `buildBlogCard(post)` + `web/src/routes/blog/[slug]/og.png/+server.ts` rendering a 1200×630 card via `renderMarkupPng`/`loadOgFonts`; unknown/draft slug → 404

## 5. RSS

- [ ] 5.1 `web/src/routes/blog/rss.xml/+server.ts`: valid RSS 2.0 from `listPosts()` (title/link/guid/pubDate/description), newest-first, drafts excluded, XML-escaped; unit-test the feed builder

## 6. Verification

- [ ] 6.1 Run `npm run build`, `svelte-check`, `vitest`, and `eslint` in `web/`; manually verify `/blog`, a post page, `/blog/<slug>/og.png`, `/blog/rss.xml`, and the post's presence in `sitemap-pages.xml`
