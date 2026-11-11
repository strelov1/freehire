**Betterteam traps** (`betterteam`; verified 2026-11-11):

- **The native slug is not always a numeric-suffixed URL token.** Modern pages
  carry a React Router `routes/CareersPage` loader with a `jobs` array. Turing's
  array held 300 records: 39 slugs contained parentheses and two had no numeric
  suffix (`markets-expert`, `medical-doctors-us`). The adapter reads those native
  slugs directly and escapes them as URL path segments. Legacy HTML listings
  retain their slug-number fallback, including percent-encoded punctuation.
- **300 is an observed ceiling, not a complete catalogue.** Turing's loader
  reported exactly 300 records without a total or continuation cursor. Reaching
  that boundary fails before detail requests, so the company-scoped sweep cannot
  treat an unproven prefix as the whole feed. Do not raise or silently accept the
  ceiling. A verified continuation API or authoritative end proof would reopen
  the completeness decision.
- **No `fullBoardListing` or `emptyBoardListing` marker.** Surplus City's native
  loader explicitly answered `jobs: []`, but the large-board ceiling prevents
  granting provider-wide complete-listing capability. Empty-only closure needs
  its own proved contract; do not infer it from a root with no matching anchors.
  A verified own-posting-URL liveness backstop is a separate path for known gone
  links and does not imply complete listing coverage.
- **Malformed native state fails.** A modern loader with missing, null or
  invalid required reference-table fields is not an empty board. Only a page
  without that modern payload uses the legacy HTML reader.
