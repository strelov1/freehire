**gr8people traps** (all verified live 2026-09-06, across `etrade.gr8people.com`,
`ardene.gr8people.com` and `batesville.workgr8.com`):

- **`gr8people.com` and `workgr8.com` are one vendor, not two.** Both career-site hosts serve the
  byte-identical Next.js "app-career-site" bundle from the same asset CDN
  (`assets.gr8people.com`), the same GraphQL schema, and the same session-token mechanism —
  confirmed by running the identical query against a tenant on each domain and getting the same
  response shape back. This is the Factorial/FactorialHR case (`factorial.go`,
  `atsboard/board_hosts_b.go`'s `factorial`/`factorialhr` rows): one adapter, one provider key
  (`gr8people`), `modeHost` on both domain labels.
- **The board's session token comes from the tenant's OWN `/jobs` page, and it is tenant-scoped.**
  A GET of `https://<host>/jobs` is server-rendered and its `__NEXT_DATA__` script embeds
  `"token":"eyJ..."` (ES256 JWT, `iss: auth-service`, ~5h lifetime measured from `iat`/`exp`).
  Decoding two tenants' tokens shows different `org` claims (`db63c8` for etrade, `0f3be7` for
  ardene, `392f45` for batesville) — the token is NOT portable across hosts, so it must be minted
  fresh per board, from that board's own host, never cached or shared.
- **The search call is GraphQL, not REST**: `POST https://<host>/graphql`,
  `Authorization: Bearer <token>`, operation `searchJobs` aliasing the schema's
  `searchJobPostings(query, start, first, after, sort, filters)` field. The bundle actually ships
  TWO backends behind the same alias — `searchJobPostings` (the vanilla path) and
  `searchGoogleJobDiscovery` (gated by a `google-job-discovery` feature flag, `false` on every
  tenant sampled) — this adapter reads the vanilla one only.
- **The query already answers only what a real visitor sees, with an EMPTY filters object.**
  Every posting returned across all three tenants was `status: "OPEN"`, `postType: "EXTERNAL"`
  with `filters: {}` (no actual filter clause set) — unlike `jobappnetwork`'s shared Elasticsearch
  proxy, there is no equivalent unscoped-read risk to guard against here, so the adapter never
  sets a filter clause of its own.
- **Pagination is Relay-style cursors (`after`/`endCursor`/`hasNextPage`), not an exact total to
  chase.** `totalCount` is reported but the walk stops on `hasNextPage: false` or an empty `nodes`
  page — confirmed by paging a 173-posting tenant end to end with no gaps or overlap. Passing an
  EMPTY STRING as `after` on the first page is rejected by the platform's cursor decoder; the key
  must be omitted entirely until a real cursor exists.
- **`places.nodes[].name` is already a formatted "City, Region[, Postal], Country" string per
  place** — there is no separate structured country-code field on the posting — so it is passed
  through as free text (the `apploi`/`hrmdirect` posture) and `Countries` is left to the location
  dictionary. `workplaceType` (`ON_SITE`/`REMOTE`/`HYBRID`) is a genuine closed enum, confirmed by
  a validator function in the platform's own bundle, so it is a real structured work-mode signal
  unlike the free-typed `positionType.name`.
- **No adapter-scale request volume was sent during this research** — like jobappnetwork, this
  entry does NOT claim "no metering observed". Ship without a pacer and watch `board_health`.
