## Why

`GET /api/v1/companies` returns `[]db.ListCompaniesRow` — the struct `sqlc` generates from a
`SELECT` — straight to the client. Its json tags *are* the public API, so `make sqlc` after a
column rename or a changed `SELECT` alias rewrites the endpoint's response with no compile error
anywhere. Every other list endpoint in the package (`agent_search.go`, `jobs.go`, `search.go`,
`swipe.go`, `recommendations.go`, `inbox.go`) serves a projection; this is the sole exception,
and it abandons the rule `internal/jobview` exists to enforce one endpoint over.

The cost is already visible in the code. The endpoint has two backends, and the Meilisearch one
has to fabricate a persistence row it has nothing to do with — `companyRowFromDoc` builds a
`db.ListCompaniesRow` field by field and `pgText` re-wraps plain strings into `pgtype.Text`
purely so JSON null-ness matches the Postgres path. Add a column to the query and the Postgres
branch grows a field the Meili branch silently omits, so the same request returns different
bodies depending on which backend served it.

Nothing is broken today. The exposure is the next `make sqlc`.

## What Changes

- A `companyListItem` struct in `internal/handler/companies.go` with plain Go types — `*string`
  for the two nullable columns — becomes the endpoint's wire shape. Verified byte-identical:
  `pgtype.Text` and `*string` marshal the same in all three states (`"x"`, `""`, `null`).
- `fromListRow(db.ListCompaniesRow)` and `fromDocument(search.CompanyDocument)` project the two
  backends onto it, and both branches return `[]companyListItem`.
- **`companyRowFromDoc` and `pgText` are deleted.** The Meili path stops imitating a database
  row; it projects onto the wire type directly, like every other search-backed surface.
- **`pgconv.TextPtr`** — found while implementing. The deleted `pgText` was a duplicate of
  `pgconv.Text`, and `pgconv` had no null-preserving read even though `TimePtr` and `IntPtr` are
  exactly that shape. Adding it there rather than a fourth local unwrapper also makes
  `internal/handler` import `pgconv` for the first time, which is what S12 asks of the package.
- No response change. The endpoint's JSON is identical before and after, which is the point: the
  contract stops being a side effect of code generation and becomes something a diff can show.
  Pinned by golden bodies captured from the OLD type before the refactor, so the proof is that
  they still pass.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `companies`: gains the rule that the list endpoint's item is a projection owned by the
  transport, not a generated persistence row — and that both backends serve the same type, so
  they cannot drift field by field.

## Impact

- `internal/handler/companies.go` — the new struct, two projections, two deleted helpers, and the
  two `listResponse` call sites. `internal/pgconv/pgconv.go` — one added converter.
- No migration, no query change, no new dependency, no frontend change.
- Deliberately NOT in scope: an `internal/companyview` package mirroring `jobview`. `jobview`
  earns a package by being shared across list, detail, search and the index; companies have one
  six-field list row and one detail view in the same file. Also out of scope: retyping
  `companyView`'s `pgtype` fields through `internal/pgconv` — that is S12's cheap follow-up and
  touches a different endpoint.
