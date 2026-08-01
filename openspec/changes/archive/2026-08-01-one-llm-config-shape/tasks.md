## 1. One shape, two policies

- [x] 1.1 `config.LLM` with `LoadLLM()` and `Require()`, the latter naming EVERY missing setting
      rather than the first.
- [x] 1.2 Embed it in `Settings` and `Enrich`. Preserve the policy split — the server must not
      gain a `Require` call, because degrading is its documented behaviour.
- [x] 1.3 `LLM.Settings(model)` as the one mapping into the library shape, with the model an
      argument because the assistant and the rest share a connection and differ only by it.

## 2. Collapse the seven literals

- [x] 2.1 All six entrypoints call it.
- [x] 2.2 `cmd/tg-extract` and `cmd/classify-mail` stop loading the ENRICHMENT config for the LLM
      half — the coupling that made them inherit `ENRICH_*` validation.

## 3. Delete what the duplication grew

- [x] 3.1 `Enrich.LangfuseEnabled` — dead, and a second answer to a question `internal/llm`
      already owns. Confirm `NewTracer`'s all-three rule is tested there before deleting.
- [x] 3.2 Replace its test with one asserting carry-through, which is config's actual job.

## 4. Verify and close

- [x] 4.1 `go test ./...` AND `go test -tags=integration ./...`.
- [x] 4.2 Confirm no `llm.Settings{…}` literal remains outside the one mapping.
- [x] 4.3 Mark #19/#34/#45 ✅ in `docs/reviews/2026-08-01-architecture-review.md`.
