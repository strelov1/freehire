# internal/platform

Infrastructure with no opinion about jobs, candidates or hiring: the generated SQL layer and pool, the cron-worker plumbing, outbox and cache primitives, HTTP and blob clients, the LLM transport, and the small shared utilities. Nothing here knows what a vacancy is.

**Layer 1 of 8.**

May import: nothing else under `internal/`. This is the bottom.

Must NOT import: `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search`, `engage`, `ingest`, `api`.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`aigateway` `arch/layering` `backfillpage` `blobstore` `cache` `config` `database` `db` `externalid` `flexjson` `htmltext` `isoweek` `linktoken` `llm` `llmschema` `migrate` `modroot` `observability` `outbox` `pgconv` `pgerr` `safehttp` `stringset` `testdb` `tokencrypt` `tracerlink` `worker`
