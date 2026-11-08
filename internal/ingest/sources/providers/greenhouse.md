**Greenhouse traps** (`greenhouse`):

- **`CompanyDescriber` fetches `boards-api.greenhouse.io/v1/boards/{board}`** (distinct from the `/jobs` listing `Fetch` calls), which carries an employer-authored "about us" `content` field for a meaningful share of boards (confirmed live: Coinbase, Figma, Asana). It is sanitized through `sanitizeHTML` with no `html.UnescapeString` (served as raw HTML, not entity-encoded).
- **Empty board** (`emptyBoardListing`): `{"jobs":[],"meta":{"total":0}}`; an unknown board 404s.
