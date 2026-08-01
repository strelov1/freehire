## Context

`GET /api/v1/companies` has two backends and one response. The Postgres branch returns
`[]db.ListCompaniesRow` from sqlc; the Meilisearch branch converts each hit into the *same
generated struct* so the two agree:

```go
rows[i] = companyRowFromDoc(h)          // search.CompanyDocument → db.ListCompaniesRow
Tagline: pgText(d.Tagline),             // string → pgtype.Text, "" meaning NULL
```

The comment on `companyRowFromDoc` states the goal plainly — "so the Meili path is byte-for-byte
compatible with the Postgres path" — and the mechanism it chose is to imitate the persistence
type. That makes `db.ListCompaniesRow`'s json tags the public contract of the endpoint, which is
the whole thing `internal/jobview` exists to prevent one endpoint over:

> One type, projected from the job.Job aggregate, so the API surfaces cannot drift apart.

Two consequences, neither firing today:

- `make sqlc` is an API-changing operation here. Rename a column or change a `SELECT` alias and
  the response changes with nothing to compile against.
- A new column in the query lands in the Postgres branch only. `companyRowFromDoc` sets fields
  one by one, so the Meili branch would omit it — and the two branches serve the same URL, so the
  body would depend on whether the caller typed a search term.

## Goals / Non-Goals

**Goals:**

- The endpoint's response shape is a type the transport owns, declared next to the handler that
  serves it, and both backends project onto it.
- Byte-identical output. This change must be provable as a no-op on the wire, or it is a silent
  API break dressed as a refactor.
- The Meili branch stops constructing a persistence value.

**Non-Goals:**

- `internal/companyview`. `jobview` earns its package by being shared across the list, the detail
  read, the search index and the wrapper responses; `companies` has one six-field list row and
  one detail projection, both in one file. A package here would be structure without a second
  consumer.
- `companyView` (the detail endpoint) keeps its `pgtype` fields. It is the same family of
  problem, but it is a different endpoint, it is already a hand-written projection rather than a
  generated row, and S12 covers the `pgconv` question directly.
- Any change to the query, the index, or the fallback rule.

## Decisions

**`*string` for the nullable columns, not `pgtype.Text` and not a bare `string`.** The wire
behaviour must not move, so the candidate had to be checked rather than assumed. Marshalled
directly:

| value | JSON |
|---|---|
| `pgtype.Text{String: "x", Valid: true}` | `"x"` |
| `pgtype.Text{String: "", Valid: true}` | `""` |
| `pgtype.Text{Valid: false}` | `null` |
| `*string` → `"x"` | `"x"` |
| `*string` → `""` | `""` |
| `*string` → `nil` | `null` |

Identical in all three states. A bare `string` would collapse `null` into `""` and change the
response for every company with no tagline.

*Alternative considered — keep `pgtype.Text` in a hand-written struct.* It removes the sqlc
coupling but keeps a persistence vocabulary on the wire, which is the half of the finding that
makes the Meili branch need `pgText` at all.

**Two named projections, `fromListRow` and `fromDocument`, rather than one generic converter.**
The two backends genuinely differ: the row already carries null-ness, the document carries `""`
for absent scalars and `nil` for absent arrays. Each conversion states its own rule, and the
empty-string-means-absent decision that `pgText` used to hide moves into `fromDocument` where a
reader looking at the search path will find it.

**The array normalisation stays exactly where it is.** `companyRowFromDoc` normalises a nil
`Industries`/`Collections` to `[]string{}` so it serializes as `[]` rather than `null`, matching
Postgres's `'{}'`. That rule is load-bearing for the same reason the null-ness is — it moves to
`fromDocument` unchanged, with its comment.

## Risks / Trade-offs

- **A silent response change would be invisible to the existing tests if they assert on the Go
  struct rather than the JSON.** → The change is pinned by a test that marshals both branches'
  output and compares the bytes, not the values. That is the only assertion that can actually
  catch a null-ness regression.
- **The struct and the query can still drift** — adding a column to `ListCompanies` will not
  automatically surface it. → That is the intended trade: the drift becomes a deliberate edit in
  the handler instead of an automatic rewrite of the public API. Nothing is lost, because a
  column nobody projected was never reaching the Meili branch either.
- **One more type to read.** → It replaces two helpers (`companyRowFromDoc`, `pgText`) whose only
  job was to make a persistence type do a transport type's work, so the file gets shorter.

## Migration Plan

None. No schema, no query, no response change. Rollback is the revert.
