## 1. Fold and plan

- [x] 1.1 Add `classify.FoldTerm` and test it (`Back-End` / `back end` to `back-end`, `ML / AI` to `ml-ai`).
- [x] 1.2 Add `classify.PlanGap`: refuse an empty fold or a term over 80 characters, increment a pending or accepted row, insert a new pending row when the only rows are rejected. Test the decision.

## 2. Queue

- [x] 2.1 Add migration `0188` for `dictionary_gaps` (kind in `specialization`, `skill`) and `dictionary_gap_daily`, including the partial unique index on a live `(kind, normalized_term)`.
- [x] 2.2 Add the sqlc queries that list rows by key, insert a pending row, and increment a live row without rewriting `raw_term` or `submitted_by`. Test them against Postgres.
- [x] 2.3 Enforce the 20-writes-per-UTC-day cap in the same transaction. The 21st write changes nothing.

## 3. HTTP

- [x] 3.1 Add `POST /api/v1/dictionary/gaps`. Cookie auth. 401 anonymous, 400 for an unknown kind or a blank or oversized term, 409 when the dictionary already resolves the term, 429 on the daily cap, otherwise `{request_count, status}`. No route lists the queue.
- [x] 3.2 Resolve `specialization` with `classify.Parse` and `skill` with `skilltag.Canonicalize`.

## 4. Page

- [x] 4.1 Give `CategoryPane` a bindable `query` and `SearchSelect` a bindable `filter`, plus an optional `empty` snippet on both. The filter modal is unchanged.
- [x] 4.2 Add `/dictionary` with a Specialization section (`CategoryPane`, plain) and a Skills section (`SearchSelect`, tech icons). Lists come from `CATEGORY_GROUPS` and the skills glossary catalog; counts from one best-effort unfiltered facet call.
- [x] 4.3 Show a "Show jobs" link to `/jobs` when chips are picked.
- [x] 4.4 When a search matches nothing, show the "not in the dictionary" line and an "Add to the list" button that writes only on click. Show "Asked N times", the 409 message, and the 429 message.
- [x] 4.5 Send an anonymous click to sign in, returning with `?specialization=` or `?skill=` seeding the search.
- [x] 4.6 Add the footer Product link to `/dictionary` immediately after Skills glossary.

## 5. Gate

- [x] 5.1 `gofmt -l .` is silent, `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...` pass. The web unit tests pass.
