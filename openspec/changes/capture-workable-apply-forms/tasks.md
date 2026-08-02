## 1. The mapper

- [x] 1.1 Add the Workable payload types and mapper to `internal/applyform`, covering the measured vocabulary (`text`, `email`, `phone`, `paragraph`, `date`, `number`, `boolean`, `file`, `dropdown`, `multiple`) and leaving an unmeasured kind unnormalized
- [x] 1.2 Cover the inverted option pair: the platform's `value` is the label a candidate reads and its `name` is the value to submit
- [x] 1.3 Cover that a field group is one control and its nested fields are not walked

## 2. The fetcher

- [x] 2.1 Add the fetcher over `apply.workable.com/api/v1/jobs/{shortcode}/form`, addressed by the posting id alone, and register `workable` so the enqueue gate and the worker both see it
- [x] 2.2 Cover that a not-found posting is marked gone rather than retried, like the other fetchers

## 3. Display

- [x] 3.1 Teach the display projection that a Workable control is an employer's question only when the platform marks it with its `QA_` prefix, and cover that the standard profile collapses

## 4. Finish

- [x] 4.1 Run `go build ./...`, `go vet ./...`, `go vet -tags=integration ./...` and both Go test passes
- [x] 4.2 Note Workable in the capture provider list in `AGENTS.md` and `internal/sources/AGENTS.md`
