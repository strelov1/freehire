# Pangian and Remote4Me as sources

This project does not crawl Pangian or Remote4Me. Neither publishes postings any more.

## Why this is out of scope

Checked live on 2026-11-18:

- **Pangian**: every pangian.com address, including the old listing (`/job-travel-remote/`) and
  `/feed/`, answers a 114-byte parking page that redirects to `/lander`.
- **Remote4Me**: the home page is a static landing page ("1000+ remote jobs from 20+ job boards")
  whose 90 job-area links all open one Google Form; `/faq` and `/about` redirect to it, `/rss` and
  `/sitemap.xml` are gone, and the only JSON-LD is the site's own Organization markup. Even when
  live it republished other remote boards, most of which are crawled here directly.

Revisit if either site serves its own job postings again.

## Prior requests

- 2026-11-18: a "20 websites to find a remote job" list, checked board by board.
