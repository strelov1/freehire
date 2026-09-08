## 1. Tests first

- [x] 1.1 In `internal/ingest/sources/http_test.go`, add a test where an `httptest` server
      answers `200` with an empty body on the first request and a valid decodable body on the
      second; assert the call succeeds and the decoded value matches the second response.
- [x] 1.2 Add a test where the server answers `200` with an empty body on every request across
      the full retry budget; assert the call fails after exactly `maxRetries+1` attempts, with
      an error that still names the URL.
- [x] 1.3 Add a test where the server answers `200` with a non-empty but malformed body (e.g.
      invalid XML) on every request; assert the call fails immediately after exactly 1 attempt
      (no retry spent on a genuine parse error).
- [x] 1.4 Run `go test ./internal/ingest/sources/...` and confirm the three new tests fail
      against the current (pre-fix) code for the right reason (1.1 and 1.2 fail because no
      retry happens; 1.3 already passes and stays a regression guard).

## 2. Implementation

- [x] 2.1 In `Client.do` (`internal/ingest/sources/http.go`), in the `resp.StatusCode >= 200
      && resp.StatusCode < 300` branch, after `err := r.decode(resp)` fails, branch on
      `errors.Is(err, io.EOF)`: on true, set `lastErr` to the wrapped decode error and
      `continue` the attempt loop (same shape as the `5xx` branch); on false, keep today's
      immediate `return fmt.Errorf("sources: decode %s: %w", r.url, err)`.
- [x] 2.2 Confirm `io` is imported (already used elsewhere in the file) and add `errors` if not
      already imported.

## 3. Verification

- [x] 3.1 `gofmt -l internal/ingest/sources/http.go internal/ingest/sources/http_test.go`
      prints nothing.
- [x] 3.2 `go vet ./...` passes.
- [x] 3.3 `go test ./internal/ingest/sources/...` passes, including the three new tests from
      section 1.
- [x] 3.4 `go test ./...` passes (full unit suite, no external deps).
