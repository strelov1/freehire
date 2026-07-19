## 1. Transport: challenge detection

- [x] 1.1 Add `ChallengeError` type (URL field, `errors.As`-matchable) in `internal/sources/http.go`, next to `StatusError`
- [x] 1.2 In `Client.do`, before the 2xx success branch, return `ChallengeError` when the response header `x-amzn-waf-action == "challenge"`, closing the body and not decoding; return immediately without consuming a retry
- [x] 1.3 Tests: challenge header → `ChallengeError` and decode fn is never called; a normal 200 still decodes; a challenge is not retried

## 2. Optional paced challenge-aware getter

- [x] 2.1 Add `pacedClinchGetter(c HTMLGetter) HTMLGetter` in `internal/sources/pacer.go`, reusing `rateLimitedHTMLGetter` with a conservative `clinchRequestInterval`/`clinchRequestBurst` (documented as tunable, per the careerspage/vagas precedent)
- [x] 2.2 Tests: the paced getter gates `GetHTML` through the shared limiter (fake `waiter`) and propagates a `ChallengeError` from the inner getter unchanged

## 3. clinch detail hydration + latch

- [x] 3.1 Add a description extractor: given the detail page's `*html.Node`, return the text of `div.job-description` (empty string when absent)
- [x] 3.2 Change `NewClinch` to take the composed getter it needs (sitemap XML + paced HTML detail); update the constructor and `clinch` struct
- [x] 3.3 In `Fetch`, hydrate each posting's `Description` via the paced `GetHTML`, with a one-way latch: on the first `ChallengeError`, stop fetching detail for the rest of the run; a non-challenge detail error leaves the posting with an empty description and continues
- [x] 3.4 Tests: 200 with `div.job-description` → description set; 200 without the block → empty description; first `ChallengeError` latches remaining postings to sitemap-only; a per-posting non-challenge error does not drop the posting; all sitemap postings are still emitted in every case (non-regression)

## 4. Wiring

- [x] 4.1 Wire clinch in `internal/sources/source.go` to construct its detail getter via `pacedClinchGetter`, mirroring how careerspage/hh/vagas are wired
- [x] 4.2 `go build ./... && go vet ./... && go test ./internal/sources/...` green

## 5. Verification

- [x] 5.1 Run the clinch adapter against a live board in isolation (respecting the pace) and confirm at least the first postings carry a non-empty description and a mid-run challenge latches cleanly (no crash, no garbage description, all postings emitted)
