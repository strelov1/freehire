**GulfTalent traps** (`gulftalent`; measured 2026-10-30):

- **It is swept by source (`fullCatalog`).** The sitemap's job shards list the whole catalogue
  (~36k postings), the listing fails on any unreadable shard or on a listing with no posting,
  and a closed posting simply drops out of the sitemap (its page still answers 200, saying
  "Job closed", with no JobPosting block). Under the company scope 12,906 of ~15k open postings
  had sat unseen since 2026-07-07 — every one sampled was closed.
- **A fixed hydration window never drains.** Most new postings belong to companies an ATS
  already covers; they are skipped after hydration, never stored, and so stay "new" every run.
  With the window fixed at the sitemap's head, 383 of each run's 500 detail pages were the same
  covered postings, day after day, while ~33.5k live postings further down were never read.
  So a run takes the newest 500 plus a backlog window of 1000 that moves every day.
