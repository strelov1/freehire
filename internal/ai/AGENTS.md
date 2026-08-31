# internal/ai

Model-backed features, the spend attribution around them, and the plan that bounds them: enrichment, embeddings, the in-app assistant, speech, plan limits. The LLM client itself is NOT here — it is transport, and lives in `platform`.

**Layer 3 of 8.**

May import: `platform`, `dict` — and itself.

Must NOT import: `identity`, `candidate`, `job`, `application`, `search`, `engage`, `ingest`, `api`.

`identity` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`aiarchetype` `assistant` `autofillagent` `browsertools` `embed` `enrich` `llmkey` `plan` `speech`
