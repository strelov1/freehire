## 1. Tech title detector

- [x] 1.1 Add `techTitleTerms` (confident software/IT role terms, whole-word) to `internal/classify/dictionaries.go` (or nontech.go's sibling)
- [x] 1.2 Add `classify.IsTech(title string) bool` with unit tests: software/IT titles → true; trap negatives (Mechanical/Sales/Project/Manufacturing Engineer, Geologist, bare "engineer"/"analyst") → false; word-boundary

## 2. Symmetric derivation

- [x] 2.1 Fold `IsTech(title)` into `jobderive.deriveIsTech`: tech category OR `IsTech(title)` → true; else non-tech → false; else nil
- [x] 2.2 Unit tests: detector-only tech title → true; tech-first precedence; Mechanical Engineer stays nil; existing non-tech/unknown cases unchanged

## 3. Verify + ship

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green
- [ ] 3.2 Deploy, run `cmd/backfill-derive` + reindex, measure new true/false/null split on prod
