## 1. ATS detection core (`internal/atsdetect`)

- [x] 1.1 Define `Detect(html string) (provider, slug string, ok bool)` with the greenhouse (direct + `embed/job_board?for=`), lever, and ashby URL patterns and a slug-shape guard; fixed provider precedence
- [x] 1.2 Unit-test detection: direct link per provider, greenhouse embed (slug not `embed`), no-match, malformed-slug rejection, multi-provider determinism

## 2. Career-page fetching

- [x] 2.1 Add a fetcher that, given a website, fetches the homepage + fixed careers/jobs paths and one homepage-discovered careers link via the `internal/sources` HTTP client (timeout, 429 backoff), returning the first HTML that `atsdetect` resolves
- [x] 2.2 Unit-test the careers-link discovery (pick the right `href` from homepage HTML) and the "first detection wins / all fail → none" logic with a fake fetcher

## 3. Dataset website extraction (`cmd/harvest-ats extract`)

- [x] 3.1 Parse `(name, website)` from each collection dataset (yc `website`, european `Website URL`, fortune500/techstars CSV columns; unicorn website when present), reusing `collections.All` dataset URLs; unit-test each parser on a fixture
- [x] 3.2 Filter out companies whose `normalize.Slug(name)` is in the supplied company-slug-set file; emit unmatched `{name, website}` JSON; unit-test the filter + website-required drop

## 4. Resolve command (`cmd/harvest-ats resolve`)

- [x] 4.1 Wire `resolve`: read the unmatched JSON, fan out over the fetcher with bounded concurrency, accumulate detected slugs per provider, write `<provider>.seed.json`; best-effort (log + skip per-company failures)
- [x] 4.2 Confirm harvest-ats is host-only (no Dockerfile entry) — parity with harvest-boards, which is also not shipped in the image

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` green; gofmt clean
- [x] 5.2 Live smoke: run `extract` + `resolve` on a small sample, confirm seeds are produced and a sample slug validates via `harvest-boards`
