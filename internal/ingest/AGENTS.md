# internal/ingest

How postings enter the catalogue: the source adapters and the crawl pipeline, ATS board recognition, link import, Telegram, apply-form capture — and the manual paths, moderator-authored vacancies and the public submission queue.

**Layer 7 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search` — and itself.

Must NOT import: `engage`, `api`.

`engage` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`adzunadesc` `applyform` `atsboard` `atsdetect` `boardresolve` `catalogstats` `contribution` `jdresolve` `linkimport` `linksource` `moderation` `pipeline` `screeninganswers` `sources` `sourcestats` `submission` `telegram`

## The service-extraction seam

`ingest` reaches into a higher block in exactly four places. They are the whole cost of
running the crawl as its own service, and they are listed here so the number stays honest:

```
linkimport -> ai/enrich          linkimport -> search/search
telegram   -> platform/llm       telegram   -> platform/llmschema
```

The two `platform` edges are transport and cost nothing — a separate binary links the same
client. The two from `linkimport` are the real seam: importing a job by URL enriches it and
pushes it to the index inline.
