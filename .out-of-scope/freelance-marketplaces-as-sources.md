# Upwork, Toptal, We Are Rosie and Gun.io as sources

This project does not crawl Upwork, Toptal, We Are Rosie or Gun.io. It reads job boards and ATS
platforms that publish vacancies; Hubstaff Talent, the one freelance marketplace on that list that
publishes a readable listing, is crawled as an ordinary source.

## Why this is out of scope

Checked live on 2026-11-18:

- **Upwork**: the job search and the old `/ab/feed/jobs/rss` feed both answer a Cloudflare block
  page (HTTP 403) to a plain client. What it lists are one-off gigs and hourly contracts, not
  vacancies.
- **Toptal**: `/freelance-jobs` and `/sitemap.xml` answer the same Cloudflare 403. Toptal is a
  closed talent network: its engagements are matched to vetted members, not published.
- **We Are Rosie**: the site has no public listing at all (`/jobs` is a 404); gigs are offered to
  members inside the product.
- **Gun.io**: `gun.io/jobs/` (WordPress) shows four rotating teaser cards ("A Series B SaaS
  company…") with no id, no company and no per-job page; every link goes to the sign-up flow on
  `app.gun.io`, where the real listing sits behind a login. The sitemaps and `wp-json` carry no job
  type.

A browser tier could pass Cloudflare, but each page costs a stealth Chromium session (see the
Scrapling tier in `internal/ingest/sources/AGENTS.md`), and what is behind the wall is not a
vacancy catalogue.

Revisit if one of them publishes a public, keyless listing of employment roles (a feed, an API or
server-rendered pages a plain client is served).

## Prior requests

- 2026-11-18: a "20 websites to find a remote job" list, checked board by board.
