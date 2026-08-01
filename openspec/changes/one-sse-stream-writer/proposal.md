## Why

Both long-lived SSE endpoints — the assistant turn and the fit analysis — had to solve the same
four problems, and solved them twice:

- fasthttp's `WriteTimeout` racing the stream writer, so a deadline set once up front loses about
  half the time and kills the stream at exactly ten seconds, mid-answer;
- a *cleared* deadline being forever, so a reader that stopped reading pins the goroutine for the
  life of the process;
- `bufio.Writer` not being safe against the keepalive ticker;
- the request-scoped Sentry hub dying before the writer runs.

One of them solved it with a small type (`sseStream`). The other copied the reasoning into an
inline closure, a bare `var mu sync.Mutex`, a hand-rolled ticker goroutine, and two package-level
free functions. This is one subtle concurrency/deadline protocol, not two vendor-shaped
implementations — the next fix to the deadline race has to be made twice, and nothing says so.

The split had already started to drift: the assistant reaches into the other file's
`sseWriteTimeout` while re-implementing the type that owns it; one endpoint names its keepalive
interval and the other writes a bare `15 * time.Second`; and `handler/AGENTS.md` still contrasts
`writeEvent` with `writeSSE`, a function that no longer exists.

## What Changes

- `sseStream.event` gains a bool result — did the frame reach the client — with a marshal failure
  reporting **true**, because an unencodable frame is our bug and must not cancel a live turn.
- The keepalive goroutine both endpoints hand-rolled moves onto the type as
  `keepalive(every) (stop func())`, where `stop` blocks until the goroutine has finished. Getting
  that ordering wrong writes into a closed body.
- `streamTurn` is rewritten over `newSSEStream`. `writeEvent`, `writeComment`, the inline write
  closure, the bare mutex and the ticker goroutine are deleted.
- `sseHeaders(c)` holds the four headers both endpoints set identically, so a new SSE endpoint
  cannot ship with three of the four.
- `assistantKeepalive` and the bare `15 * time.Second` become one `sseKeepalive`.
- The Sentry hub clone stays inline in both, where its comment lives.

No behaviour change: same frames, same deadlines, same cancellation.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. `tasks.md` is the real artifact; the change
archives with `--skip-specs`.

## Impact

- `internal/handler/match_analysis_stream.go` — the type grows `keepalive`, `sseHeaders` and the
  bool result.
- `internal/handler/assistant.go` — ~50 lines of stream machinery become ~20; three imports go.
- `internal/handler/AGENTS.md` — the passage naming a function that no longer exists.
