# Tasks

## 1. Non-tech description detector (internal/classify)

- [x] 1.1 Write failing unit tests for `classify.NonTechFromDescription`: positive role-statement anchors resolve the right category (sales/marketing/support/management), and negatives return "" — incidental prose ("work with our sales team", "our support engineers"), bare words alone, tech-adjacent forms ("sales engineer", "solutions engineer"), and engineering/product/project/data manager forms.
- [x] 1.2 Implement `NonTechFromDescription(desc string) string` in `internal/classify/description.go`: an ordered slice of `{category, phrases[]}` matched via `wordmatch.Contains` with `UnicodeBoundary`, first match wins, else ""; returns only a member of `enrich.NonTechCategories`. Make the tests pass.

## 2. Wire the description tier into category derivation

- [x] 2.1 Write a failing `jobderive` test: the description tier fills `category` only when the structured source and title dictionary are both empty (structured wins; title beats description; a title-silent tech description stays empty).
- [x] 2.2 Add the third tier to the `category` precedence in `jobderive.Derive` (`structured → title → NonTechFromDescription`) and replace the stale "Description prose is too noisy…" comment. Make the test pass.
- [x] 2.3 Apply the same description fallback at the two `cmd/tg-extract` derive sites (which call `classify.Parse` directly), keeping ingest and telegram consistent.

## 3. Verify

- [x] 3.1 Run `go build ./... && go vet ./... && go test ./...` — all green; confirm the enqueue gate, schema, and served-facet doctrine are untouched (no changes outside classify/jobderive/tg-extract).
