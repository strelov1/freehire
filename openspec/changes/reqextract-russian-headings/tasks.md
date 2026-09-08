## 1. Measure against real data

- [x] 1.1 Sample real `tbank.ru` postings from the local dev DB and count real heading occurrences (`<h1>`–`<h6>` text frequency across 500 postings, 369 non-templated).
- [x] 1.2 For the candidate "Требования" heading specifically, check what follows it (list vs paragraphs) across every real occurrence — this is what actually decides whether the vocabulary addition extracts anything.
- [x] 1.3 Get the exact `unidecode.Unidecode` transliteration for each candidate phrase by running the library directly, not by hand-transliterating.

## 2. Vocabulary

- [x] 2.1 Add "trebovaniia" and "my zhdem ot vas" to `requiredHeadings` (`internal/job/reqextract/reqextract.go`), each with a doc comment citing the measured occurrence count and source.
- [x] 2.2 Add "my predlagaem" and "usloviia" to `closingHeadings`, same doc-comment convention.
- [x] 2.3 Confirm "Обязанности" is deliberately excluded (not a required heading) and note why in the vocabulary's own context, matching the existing "what you'll do" exclusion for English.

## 3. Tests

- [x] 3.1 `TestDerive` case: a Russian requirements heading followed by a real `<ul>` list extracts its items — proves the vocabulary addition works.
- [x] 3.2 `TestDerive` case: a Russian requirements heading followed by `<p>` paragraphs (the real measured `tbank.ru` shape) yields nothing — an honest negative case in the suite itself, not only in prose.
- [x] 3.3 Full `reqextract` package test suite passes.

## 4. Documentation

- [x] 4.1 `internal/job/reqextract/AGENTS.md`: reframe the language-coverage Limitations bullet (Hungarian AND Russian, with the measurement discipline spelled out).
- [x] 4.2 Add a new Limitations bullet for the `<p>`-vs-list structural gap this investigation surfaced, explicitly scoped as separate, larger, not fixed here.

## 5. Wrap-up

- [x] 5.1 `go vet -tags=integration ./...` and the full test suite for `internal/job/reqextract`.
- [x] 5.2 `gofmt -l .`, `go vet ./...`, `go test ./...` clean before commit.
