## 1. Pin the wire shape before touching it

- [x] 1.1 RED-by-construction: add a test that marshals the SAME company through both backends'
      projections and asserts the two JSON bodies are byte-identical, including `null` tagline
      and `[]` industries. Write it against the shape as it will be, so it compiles only after
      the type exists.
- [x] 1.2 Capture today's exact JSON for a fully-populated company and for an all-empty one, and
      assert the new type reproduces both byte for byte. This is the only assertion that can
      catch a null-ness regression; comparing Go values cannot.

## 2. The projection

- [x] 2.1 Add `companyListItem` to `internal/handler/companies.go` with `*string` for `tagline`
      and `hq_country` — verified to marshal identically to `pgtype.Text` in all three states —
      and the same json tags the generated row carries.
- [x] 2.2 `fromListRow(db.ListCompaniesRow) companyListItem`.
- [x] 2.3 `fromDocument(search.CompanyDocument) companyListItem`, carrying the two rules `pgText`
      and `companyRowFromDoc` used to hold: empty string means absent, and a nil array
      normalizes to `[]` so it does not serialize as `null`.
- [x] 2.4 Both branches return `[]companyListItem`; delete `companyRowFromDoc` and `pgText`.
- [x] 2.5 Found while implementing: the deleted `pgText` was a duplicate of `pgconv.Text`, and
      `pgconv` has no null-preserving read (`TextString` collapses NULL to `""`) though its
      `TimePtr`/`IntPtr` siblings are exactly that shape. Add `pgconv.TextPtr` there rather than
      a fourth local unwrapper, and have the handler import `pgconv` — which is what S12 asks
      the package to start doing.

## 3. Verify and close

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 3.2 Confirm nothing else in the package still serves a `db.*Row`: grep every
      `listResponse` call site.
- [x] 3.3 Check the SPA's companies types against the response — no change expected, so any
      diff means the projection moved something.
- [x] 3.4 Mark S9 ✅ in `docs/reviews/2026-08-01-architecture-review.md` — shortlist row, the
      `S9` heading, the Progress table — noting anything the finding got wrong.
