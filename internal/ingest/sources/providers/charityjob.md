**CharityJob** (`charityjob`; measured from host2 on 2026-11-22):

- **One boardless catalogue of public UK charity-sector vacancies.** The unfiltered
  search is `/jobs`, then `/jobs?page=N`, with 15 cards per full page. All 115
  pages returned HTTP 200 and held 1,716 distinct posting IDs, matching the
  advertised total. Page 115 held six cards and no next link. The visible
  `Page N of M` summary identifies the actual page and its end. Starting on page 5,
  visible next-page links used `rel=nofollow`. The document head continued naming
  the next page. The adapter reads the public listing and its own posting URLs. A failed or
  unexpected listing page fails the crawl, including a page with a changed page
  count or no new open IDs. The standard unseen sweep grace accounts for listing drift.
- **Robots and transport.** The wildcard rules returned HTTP 200 and allow
  `/jobs` and its posting pages. The feeds, attachments and interview paths are
  disallowed and unused. Direct listing and sitemap requests were refused. The
  configured crawl-host browser transport with no proxy returned the listing
  and all four sampled details. Listing pages were 298,685–331,523 bytes and
  took 5.64–6.98 seconds each (5.85 median, 680.02 seconds summed across requests).
  The adapter shares a five-second start interval
  and uses one request at a time for listing and detail.
- **Posting identity and body.** The card's `job-id` must match its link in the
  form `/jobs/<organisation>/<title>/<id>` on CharityJob. The detail's
  `JobPosting.url` must name that same ID. The JSON-LD `identifier.value` belongs
  to the employer; two sampled values were `Lead Hub Worker` and `3064`.
  All four sampled bodies were in JSON-LD, 3,303–5,671 UTF-8 bytes, in detail pages
  of 282,864–289,456 bytes. They returned in 5.68–5.78 seconds. HTTP 404 or 410
  confirms closure. An unusable detail retains the listed identity while a
  complete detail outage fails the crawl.
- **Other fields.** Read title, employer and address from JSON-LD, along with
  the published date, inclusive closing day, GBP salary and an unambiguous
  full- or part-time schedule. `TELECOMMUTE` and applicant country identify
  remote work. The location label on the detail distinguishes hybrid and
  on-site roles. Technical classification uses the normal dictionaries.
- **Hydration budget.** Seen postings are refreshed by identity. A run reads
  up to 250 bodies, including a 50-posting rotating backlog lane. At roughly
  six seconds per request the measured listing plus a full hydration budget
  would take about 36 minutes based on the sampled request timings.
