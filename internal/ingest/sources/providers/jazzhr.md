**JazzHR traps** (`<board>.applytojob.com`; checked 2026-11-11):

- **A live posting need not emit `JobPosting` JSON-LD.** Four of the 14 current
  `gracefederalsolutionsllc` listing details omitted it, despite answering 200 with
  the full title, description and application form. For example,
  `/apply/33zSdUALQa/Food-Service-Worker-II` carries only Organization JSON-LD.
  The DOM fallback reads `.job-header h2`, the header's `[title="Location"]`, and
  `#job-description`, and requires the body's exact `job-<native-id>` class.
  Missing title/body or a different posting identity stays unreadable.
- **An unreadable detail withholds the board's sweep permission.** In the November
  11 crawl, the two shards recorded 16,738 and 14,278 unreadable details alongside
  25,180 and 27,733 ingested jobs. Their successful board-health records did not
  prove they read everything listed. The ordinary `boardReadWhatItListed` gate
  correctly refused to close stale rows on those boards; do not remove it.
  After deploying the DOM fix, a narrow `ingest jazzhr --board=<board>` run can
  qualify the board and close postings unseen for more than 48 hours. A detail
  404 remains positive removal evidence, while a transport or parse failure stays
  an unreadable marker.
