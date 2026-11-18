# TalentBrew

The board identity is the employer's careers host, optionally with a locale path.
The results endpoint returns JSON containing a `results` HTML fragment. Its
`section#search-results` declares `data-total-pages` on every page. ALDI US was
verified on 2026-11-11 with 17 pages; its total-record counters can differ across
pages, so they are not a completeness contract.

Pagination must finish before any posting is emitted. Returning earlier positive
pages after a later request failed previously admitted incomplete evidence to the
normal company-scoped 48-hour sweep. Missing or invalid pagination metadata,
changed page counts, empty/repeated pages in a multi-page listing, and declarations
above the 100-page bound now fail the board through the existing error path.
A declared single empty page still succeeds. This adapter declares neither full
board coverage nor verified-empty evidence; this fix does not broaden sweep scope.

Verified 2026-11-15: SC Johnson's live job 102078701904 has no JobPosting
JSON-LD. Its identified `section[data-selector-name=jobdetails]` holds
`data-job-id`, an h1, `.ats-description` and `.ajd_header__location`. The adapter
falls back to those fields only when the section's ID matches the listed posting
and both title and body are present. Dates are left absent when HTML does not
state them. A missing structured-data block does not establish removal.
