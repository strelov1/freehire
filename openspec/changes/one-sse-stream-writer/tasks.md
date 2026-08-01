## 1. Let the type answer the question the callers ask

- [x] 1.1 `event`/`write` return whether the frame reached the client; a marshal failure reports
      true, so a caller that cancels on false cannot be made to cancel by our own bug.
- [x] 1.2 Move the keepalive onto the type. `stop` must block until the goroutine has finished —
      the property worth pinning, since both endpoints hand-rolled the ticker/WaitGroup pairing.
- [x] 1.3 Take the interval as an argument rather than reading the constant: not for symmetry,
      but because the synchronous-stop property is otherwise untestable at 15 seconds.

## 2. Collapse the second implementation

- [x] 2.1 Rewrite `streamTurn` over `newSSEStream`; delete `writeEvent`, `writeComment`, the
      inline write closure, the bare mutex and the ticker goroutine.
- [x] 2.2 `sseHeaders(c)` for the four identical header lines.
- [x] 2.3 Fold `assistantKeepalive` and the bare `15 * time.Second` into one `sseKeepalive`.
- [x] 2.4 Leave the Sentry hub clone inline in both — its comment explains a request-scoped
      lifetime that is per-endpoint, not shared machinery.

## 3. Verify and close

- [x] 3.1 Tests for the two properties a reader cannot check by eye: `event` telling a dead
      reader apart from an unencodable frame, and `stop` returning only after the last beat.
      Placed beside the existing `TestSSEStream_*` rather than in a second file.
- [x] 3.2 `go build ./... && go vet ./... && go test ./...` green, plus `-race` over the SSE and
      assistant tests — the change moves a mutex, so the race detector is the relevant check.
- [x] 3.3 Fix `handler/AGENTS.md`, which contrasted `writeEvent` with a `writeSSE` that no longer
      exists, and state which endpoint ignores the disconnect signal and why.
- [x] 3.4 Mark S13 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
