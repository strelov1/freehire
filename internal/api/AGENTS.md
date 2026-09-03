# internal/api

The HTTP edge — handlers, middleware, realtime transport, OG images. The only block nothing else may import.

**Layer 8 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search`, `engage`, `ingest` — and itself.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`atsapply` `candidateprofile` `handler` `ogimage` `ratelimit` `realtime`
