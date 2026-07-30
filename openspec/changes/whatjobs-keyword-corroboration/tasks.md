## 1. Keyword corroboration

- [x] 1.1 Add `whatjobsCorroborationTerms(keyword)` returning the keyword's words minus the generic role words; test that `rust developer` → `[rust]`, `golang` → `[golang]`, and a wholly generic `developer` → empty
- [x] 1.2 Drop a posting whose title and description contain none of the corroborating terms; test that `Senior CT Technologist` under keyword `rust developer` is dropped while `Senior Rust Engineer` is kept
- [x] 1.3 Match case-insensitively as a substring so `iOS` corroborates `ios` and `Node.JS` corroborates `node.js`
- [x] 1.4 Pass postings unfiltered when the keyword has no corroborating term

## 2. Relevance-collapse pagination

- [x] 2.1 Stop crawling a keyword when a page's corroborated share falls below 50%, keeping that page's corroborated postings; test a 100%-then-15% pair stops after page 2
- [x] 2.2 Keep crawling while pages corroborate above the threshold, still ending on the empty page
- [x] 2.3 Log which condition ended a crawl — collapse, empty page, or budget — so a bounded read is never mistaken for full coverage

## 3. In-flight cap

- [x] 3.1 Add a whatjobs in-flight cap to `internal/sources/pacer.go` reusing `concurrencyLimitedJSONGetter`, documented with the 429 evidence
- [x] 3.2 Wire it in `sources.All` so all boards of a run share one semaphore

## 4. Verification

- [x] 4.1 `go build ./... && go vet ./... && gofmt -l . && go test ./...` all clean
- [x] 4.2 Live-run two keywords against the real feed; confirm pages requested drops and every returned posting names its term
- [x] 4.3 Re-annotate `sources/whatjobs.yml` volumes from corroborated counts, replacing the feed's inflated `total`
- [ ] 4.4 Deploy, run the board on prod, verify the rows are relevant before considering cron
