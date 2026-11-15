**RWB traps** (`rwb`, verified 2026-11-15):

- The listing returns at most 50 items even when `range.limit` echoes a requested 200. Advancing by 200 skips vacancies. Request and advance by 50.
- Operator's direct Scrapling HTTP lane returned upstream 498. Browser/direct returned JSON for offsets 0, 50 and 200 and a real body for vacancy 41107. Use the browser route only with the pagination fix. The separate route patch is for the integration owner of `scraplingtier.go`.
- Verification was bounded: no full listing or detail crawl. The exact adapter parser replay read two captured pages and hydrated one captured detail. A successful sample does not prove sustained capacity.
- The existing adapter skips failed details and does not claim `fullBoardListing`. The transport must preserve upstream status and service errors; do not turn a refusal into an empty JSON success.
