## 1. Fold and plan

- [x] 1.1 Add `classify.FoldTerm` and test it (`Back-End` / `back end` to `back-end`, `ML / AI` to `ml-ai`).
- [x] 1.2 Add `classify.GapTerm`: refuse an empty fold or a term over 80 characters (counted in characters, not bytes). Test it with a Cyrillic term.

## 2. Queue

- [x] 2.1 Add migration `0188` for `dictionary_gaps` (kind in `specialization`, `skill`) and `dictionary_gap_daily`, including the partial unique index on a live `(kind, normalized_term)`.
- [x] 2.2 Add `RecordDictionaryGap`, one upsert on the partial unique index: insert a pending row, or bump a live row without rewriting `raw_term` or `submitted_by`; a rejected row is not live, so a new pending row opens. Test it against Postgres, including concurrent first asks.
- [x] 2.3 Enforce the 20-writes-per-UTC-day cap in the same transaction. The 21st write changes nothing.

## 3. HTTP

- [x] 3.1 Add `POST /api/v1/dictionary/gaps`. Cookie auth. 401 anonymous, 400 for an unknown kind or a blank or oversized term, 409 when the dictionary already resolves the term, 429 on the daily cap, otherwise `{request_count, status}`. No route lists the queue.
- [x] 3.2 Resolve `specialization` with `classify.Parse` and `skill` with `skilltag.Canonicalize`.

## 4. Page

- [x] 4.1 Give `CategoryPane` a bindable `query` and `SearchSelect` a bindable `filter`, plus an optional `empty` snippet on both (and through `FacetSection`).
- [x] 4.2 Add `/dictionary` with a Specialization section (`CategoryPane`, plain) and a Skills section (`SearchSelect`, tech icons). Lists come from `CATEGORY_GROUPS` and the skills glossary catalog; counts from one best-effort unfiltered facet call.
- [x] 4.3 Show a "Show jobs" link to `/jobs` when chips are picked.
- [x] 4.4 When a search matches nothing, show the "not in the dictionary" line and an "Add to the list" button that writes only on click. Show "Asked N times", the 409 message, and the 429 message.
- [x] 4.5 Send an anonymous click to sign in, returning with `?specialization=` or `?skill=` seeding the search.
- [x] 4.6 Add the footer Product link to `/dictionary` immediately after Skills glossary.
- [x] 4.7 In the job filter modal, link the Role and Skills panes to `/dictionary` ("Can't find your role/skill? Help us recognize it"), and hand a search that matches nothing over as `?specialization=` or `?skill=`; `?tab=skill` opens the Skills tab.

## 5. Gate

- [x] 5.1 `gofmt -l .` is silent, `go vet ./...`, `go test ./...`, and `go vet -tags=integration ./...` pass. The web unit tests pass.
