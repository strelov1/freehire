**Teamtailor traps** (`teamtailor`):

- **The listing walk is hand-rolled and earns `fullBoardListing`.** It retries `/jobs`→root on a 404 and then follows the path it landed on. A later-page failure, or reaching its page cap without a genuinely empty page, fails `Fetch`; the empty-page proof is the RAW per-page item count, read before cross-page dedup.
- **The page cap once truncated a live board.** A probe against real prod boards found `ttMaxPages=100` silently truncating a real one — `tantor.teamtailor.com` (2000 open postings in the catalogue) still returned full listing pages at 100 and 101, with its true end only past page 120, a confirmed ~20-25% under-collection on a live board — closed by raising the cap to 1000 (`openspec/changes/teamtailor-listing-cap-fix`).
