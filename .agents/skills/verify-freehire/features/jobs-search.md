# Job feed and search

A visitor lands on `/jobs`, sees a feed of postings, narrows it with a keyword in the
header search box, and opens one posting on its own page.

## Sub-features

- `feed` renders job cards from the search index with a total.
- `keyword` runs a free-text search from the header box; the URL carries `?q=`.
- `open` opens a posting's page (`/jobs/<slug>`) with its title as the H1.

## How to get to it (user POV)

- Open `/jobs` (also the "Jobs" link in the top bar).
- Type in the header box (`combobox "Search jobs"`) and press Enter. Typing alone does not search.
- Click a card's title link.

## Driving it with Playwright

Preconditions: baseline; the seed indexed postings (`doctor`: `search has seeded postings`).

- **Feed.** `goto('/jobs')`. `getByTestId('job-card')` is visible; the page says `<n> jobs`.
- **Search.** Fill `getByRole('combobox', { name: 'Search jobs' })` with `engineer`, press Enter.
  URL matches `?q=engineer`, cards are visible and no more than before.
- **Open.** Click `a[href^="/jobs/"]` in the first card. The posting page (maybe a new tab)
  has URL `/jobs/<slug>` and a level-1 heading with the title.
- **Proof.** Run `run-journey.sh journeys/jobs-search.spec.ts`; evidence
  `jobs-search-1-feed.png`, `-2-search.png`, `-3-job-page.png`.

## Gotchas

- Typing before hydration is wiped: wait for `networkidle` first.
- The feed opens postings in a new tab on desktop; follow the new page.
- Without Meilisearch `/jobs` answers 500; `doctor` catches it.
- The index holds fewer postings than ingest wrote (79 of 87 on 2026-11-07): rows without a
  category or body are withheld by design, not lost.
