## Why

Catalogue findings **#19, #34 and #45** — three independent reviewers landed on the same thing.
Two config types read the same six env keys: `config.Settings` (the server) and `config.Enrich`
(the enrichment worker). Adding or renaming one LLM setting meant editing two struct definitions,
two loaders, and **seven** field-by-field `llm.Settings{…}` literals at the entrypoints.

The redundancy had already produced two concrete costs:

- `Enrich.LangfuseEnabled()` was dead — nothing outside its own test called it, because
  `internal/llm` decides tracing itself (`NewTracer` returns nil unless all three are set, which
  its own test already pins). A second answer to a question the library owned.
- **False coupling with a real failure mode.** `cmd/tg-extract` and `cmd/classify-mail` called
  `config.LoadEnrich()` purely to reach the six LLM values — they touch none of
  `Concurrency`/`LeaseSeconds`/`MaxAttempts` — so a Telegram extractor and a mail classifier
  inherited the enrichment worker's `ENRICH_*` validation and would break if it changed.

## What Changes

- `config.LLM` holds the six values, with `LoadLLM()` (permissive) and `Require()` (names every
  missing setting at once, so one run fixes the configuration rather than three).
- Both `Settings` and `Enrich` embed it. **The policy difference is preserved deliberately** — it
  is the reason the two structs existed: the server degrades when the LLM is unconfigured and so
  never calls `Require`; a worker whose whole job is to spend LLM calls does.
- `LLM.Settings(model)` is the one mapping into `llm.Settings`. The model is an argument because
  two clients share one connection and differ only by it (the assistant on `ASSISTANT_MODEL`).
  The seven literals become one call each.
- `cmd/tg-extract` and `cmd/classify-mail` load their own `config.LoadLLM()` and drop the
  enrichment dependency.
- `Enrich.LangfuseEnabled` and its test are deleted; the replacement test asserts what config is
  actually responsible for — carrying the three values through, whole or empty.

**`llm.Settings` is deliberately left alone.** The finding's own verifier is right that it is not
a third copy: it is the env-free shape the library takes so `internal/llm` depends on no
configuration package. Folding it in would break that split to remove a mapping that should exist.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. The same env keys are read with the same
strictness per caller; `tasks.md` is the real artifact and the change archives with
`--skip-specs`.

## Impact

- `internal/config` (a new `llm.go` + tests, `config.go`, `enrich.go` and their tests).
- Six entrypoints: `cmd/server` (×2), `cmd/enrich`, `cmd/tg-extract`, `cmd/classify-mail`,
  `cmd/backfill-resume-structured`, `cmd/backfill-experience`.
- Net −93 lines. No env key changes: every variable is read exactly as before.
