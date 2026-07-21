## 1. Sentinel error

- [x] 1.1 In `cmd/harvest-boards/prober.go`, change the `errMissing` string from `"not found"` to `"harvest: not found"`; confirm `go test ./cmd/harvest-boards/` stays green.

## 2. jobreality boolean fields

- [x] 2.1 Rename `jobreality.Input.EvergreenText` → `HasEvergreenText` in `internal/jobreality/classify.go` and update the `jobview/reality.go` producer, the doc comment, and `classify_test.go` references; `go build ./... && go test ./internal/jobreality/ ./internal/jobview/`.
- [x] 2.2 Rename `jobreality.Evidence.FakeFreshness` → `IsFakeFreshness` in `internal/jobreality/classify.go`, update the `jobview.Reality` consumer in `internal/jobview/reality.go` (keep JSON tag `fake_freshness`), and the tests in both packages; verify build + tests.

## 3. handler inbox filter

- [x] 3.1 Rename `inboxFilters.Unread` → `IsUnread` in `internal/handler/inbox.go` and its three references; `go build ./... && go test ./internal/handler/`.

## 4. Verify

- [x] 4.1 Run `go build ./... && go vet ./... && go test ./...`; confirm no residual references to the old identifiers and that emitted JSON keys are unchanged.
