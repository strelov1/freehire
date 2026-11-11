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
