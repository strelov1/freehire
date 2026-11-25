# weworkremotely

Verified live 2026-11-24. `/remote-jobs.rss` is capped at the newest 100 items; `?page=2` is ignored. The ten
category feeds (`/categories/<slug>.rss`, listed in the footer of `/remote-jobs`) are not capped: together they
held 1070 distinct postings (sales-and-marketing alone 293), and the all-jobs feed is a subset of them. The
crawl reads the all-jobs feed plus the ten and de-duplicates by slug (a posting can sit in two categories).
A failed feed fails the crawl: a half-read set would make every posting of the missed category look gone.

- Not mapped, on purpose: `<country>`/`<state>` (the EMPLOYER's place, often contradicting `<region>`), `<expires_at>`
  (a listing lapse, not an application deadline), `<skills>`/`<category>` (free text, not our vocabularies).
- `sitemap.xml` is behind a Cloudflare 403 for plain clients; the feeds are not. robots.txt allows `/`.
