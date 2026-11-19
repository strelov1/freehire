**DSML Kazakhstan traps** (`dsml`; live-verified 2026-11-18 from a laptop; 40 postings, Almaty, Astana and remote):

- **Boardless aggregator over `sitemap.xml`** (shared body in `sitemapboard.go`). The sitemap is the whole site; a posting is `/jobs/<uuid>`, and `/jobs/<city>` and `/jobs/<category>` pages beside them are not postings, so the id pattern is the full uuid.
- **Open to search crawlers.** robots.txt disallows `/api/`, `/auth` and account pages only; its `Content-Signal` forbids AI training, not search. The adapter paces one request every two seconds.
- **The JobPosting is the whole posting**, with `baseSalary` in the posting's own currency (KZT, USD and RUB seen; `MONTH`, `YEAR` and `HOUR`). Remote postings state `jobLocationType: TELECOMMUTE` and a null `jobLocation`; they fall back to `kz`, the community's market.
- **Most postings come from the community's Telegram channels** (`@ml_jobs_kz`, `@it_jobs_kz`). If those are ever added to `telegram_channels`, the dedup pass folds the copies. Needs a `boards` row (`cmd/add-board --provider=dsml`, no board id) to be scheduled.
