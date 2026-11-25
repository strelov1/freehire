## 1. The parser

- [x] 1.1 Add the Lever form parser to `internal/applyform`, reading each `li.application-question` block: the label, the controls grouped by submit name, and the required flag
- [x] 1.2 Cover the control shapes the real pages use — text, textarea, select with options, radio group as one control with its alternatives, file, and the consent checkbox
- [x] 1.3 Cover that the required glyph is stripped from the question text while the flag is recorded, and that a block yielding no control is not captured

## 2. The fetcher

- [x] 2.1 Add an HTML method to the transport role and the Lever fetcher over the apply page, choosing the regional host; register `lever` so the enqueue gate and the worker both see it
- [x] 2.2 Cover that a not-found posting is marked gone rather than retried, like the other fetchers

## 3. Display

- [x] 3.1 Teach the display projection that a Lever control is an employer's question only when its submit name says so, and cover that the standard application collapses

## 4. Finish

- [x] 4.1 Run `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` and both Go test passes
- [x] 4.2 Note Lever in the capture provider lists in `AGENTS.md` and `internal/sources/AGENTS.md`
