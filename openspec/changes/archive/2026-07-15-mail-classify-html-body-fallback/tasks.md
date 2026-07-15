## 1. readableBody helper (behavioral core)

- [x] 1.1 RED: unit-test `readableBody(text, html)` for all four spec scenarios —
      non-whitespace text preferred; whitespace-only text falls back to stripped
      HTML; HTML-only yields readable text (no tags); both-empty yields "".
- [x] 1.2 GREEN: implement `readableBody` in `internal/maillink/body.go` (prefer
      trimmed text, else `html2text.FromString`, raw html on strip error, "" when
      both empty). REFACTOR + simplify under green.

## 2. body_html plumbing to the classifier

- [x] 2.1 Verify claim query returns `e.body_html`, `Claimed.BodyHTML` is mapped
      by the store, and the runner passes `readableBody(c.Body, c.BodyHTML)`.
- [x] 2.2 RED→GREEN: runner test proving an HTML-only email reaches the classifier
      with the stripped body (fake classifier captures `Input.Body`) and that its
      rejection signal is what gets persisted, not screening.

## 3. Dependency + verification

- [x] 3.1 Promote `github.com/jaytaylor/html2text` from indirect to direct in
      `go.mod`; `go mod tidy`.
- [x] 3.2 `go build ./... && go vet ./... && go test ./internal/maillink/...
      ./internal/mailclassify/... ./cmd/classify-mail/...` all green.
