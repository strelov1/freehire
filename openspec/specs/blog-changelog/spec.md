# blog-changelog Specification

## Purpose

A markdown-file-backed changelog blog in the SvelteKit frontend. Posts are `.svx`
files committed under `web/src/posts/`, compiled at build time via mdsvex, with
typed frontmatter (including a `type` of `changelog` or `article`) that drives one
`/blog` feed and its post pages, SEO metadata (meta/OG/`Article` JSON-LD + sitemap
inclusion), and an RSS feed. It gives the product a public, SEO-indexable voice for
release notes and longer write-ups.

## Requirements

### Requirement: Posts are authored as frontmatter markdown files in the repo

The system SHALL source blog posts from markdown files committed under a fixed content directory in the web app, each carrying typed frontmatter, compiled at build time via mdsvex. Frontmatter fields are: `title` (string, required), `date` (ISO `YYYY-MM-DD`, required), `summary` (string, required), `type` (enum `changelog` | `article`, optional, default `changelog`), `tags` (string array, optional, default empty), and `draft` (boolean, optional, default false). The post `slug` SHALL be derived from the file name, not a frontmatter field. A `type` value outside the enum SHALL fail the build.

#### Scenario: Well-formed post file is discovered

- **WHEN** a markdown file with all required frontmatter fields exists in the content directory
- **THEN** the post is available to the blog with its slug taken from the file name and its frontmatter parsed into the typed shape

#### Scenario: Post missing a required frontmatter field fails the build

- **WHEN** a post file omits a required frontmatter field (`title`, `date`, or `summary`)
- **THEN** the build fails with an error naming the offending file, rather than shipping a post with empty metadata

### Requirement: Blog index lists published posts newest-first

The system SHALL serve a `/blog` page that lists every published post ordered by `date` descending, showing each post's title, date, summary, `type`, and tags, with the title linking to the post's page. Posts with `draft: true` SHALL be excluded from the index. The page SHALL offer a type filter with All / Changelog / Articles views; the default view SHALL show all published posts.

#### Scenario: Published posts are listed in order

- **WHEN** a user visits `/blog` and multiple published posts exist
- **THEN** the page renders one entry per published post, newest `date` first, each linking to `/blog/<slug>` and showing its type

#### Scenario: Drafts are hidden from the index

- **WHEN** a post has `draft: true`
- **THEN** it does not appear in the `/blog` listing

#### Scenario: Type filter narrows the list

- **WHEN** a user selects the Changelog (resp. Articles) filter
- **THEN** only published posts whose `type` is `changelog` (resp. `article`) are shown, still newest-first

### Requirement: Post page renders the article with SSR

The system SHALL serve `/blog/<slug>` rendering the post's compiled markdown body as HTML, server-side, with its title and date. An unknown slug SHALL return a 404. A `draft: true` post SHALL NOT be reachable at its slug in production builds.

#### Scenario: Existing post renders

- **WHEN** a user requests `/blog/<slug>` for a published post
- **THEN** the server responds 200 with the article's title, date, and rendered markdown body

#### Scenario: Unknown slug is a 404

- **WHEN** a user requests `/blog/<slug>` for a slug with no matching post file
- **THEN** the server responds 404

#### Scenario: Draft is not reachable in production

- **WHEN** a user requests the slug of a `draft: true` post in a production build
- **THEN** the server responds 404

### Requirement: Post pages carry SEO metadata

The system SHALL emit, for each post page, a `<title>` and meta description from the post's `title`/`summary`, Open Graph tags (title, description, type `article`, url, image), and `Article` JSON-LD (headline, datePublished, description, url). The Open Graph image SHALL be a per-post 1200×630 card served at `/blog/<slug>/og.png`, reusing the existing OG rendering primitives.

#### Scenario: Post page exposes metadata and JSON-LD

- **WHEN** a crawler fetches a published post page
- **THEN** the response head contains the post-specific `<title>`, meta description, Open Graph tags, and an `Article` JSON-LD block

#### Scenario: Per-post OG image renders

- **WHEN** `/blog/<slug>/og.png` is requested for a published post
- **THEN** a 1200×630 PNG card for that post is returned; an unknown or draft slug returns 404

### Requirement: Published posts appear in the sitemap

The system SHALL include `/blog` and each published post's `/blog/<slug>` URL in the static-pages sub-sitemap (`sitemap-pages.xml`). Draft posts SHALL be excluded.

#### Scenario: Sitemap includes the blog and its posts

- **WHEN** `sitemap-pages.xml` is generated
- **THEN** it contains a `<url>` for `/blog` and one for each published post, and none for draft posts

### Requirement: RSS feed of published posts

The system SHALL serve `/blog/rss.xml`, a valid RSS 2.0 feed of published posts newest-first, each item carrying the post's title, link, publication date, and summary. Draft posts SHALL be excluded.

#### Scenario: Feed lists published posts

- **WHEN** a reader fetches `/blog/rss.xml`
- **THEN** a valid RSS 2.0 document is returned with one `<item>` per published post, newest first, and no draft posts
