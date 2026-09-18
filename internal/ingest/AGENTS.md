# internal/ingest

How postings enter the catalogue: the source adapters and the crawl pipeline, ATS board recognition, link import, Telegram, apply-form capture — and the manual paths: moderator-authored vacancies, the public submission queue, and a verified employer publishing its own company's vacancies directly (see [employer/AGENTS.md](employer/AGENTS.md)).

**Layer 7 of 8.**

May import: `platform`, `dict`, `ai`, `identity`, `candidate`, `job`, `application`, `search` — and itself.

Must NOT import: `engage`, `api`.

`engage` share this layer, and the ban runs both ways: two blocks that can see each other are one block under two names.

Both directions are enforced. `depguard` in `.golangci.yml` fails on the
offending import line; `internal/platform/arch/layering` holds the same table and
reports the whole graph at once, including imports that exist only in test files.

## Packages

`adzunadesc` `applyform` `atsboard` `atsdetect` `boardresolve` `catalogstats` `contribution` `employer` `jdresolve` `linkimport` `linksource` `moderation` `pipeline` `screeninganswers` `sources` `sourcestats` `submission` `telegram`

## The service-extraction seam

`ingest` reaches into a higher block in exactly five places. Four are the whole cost of
running the CRAWL as its own service, and are listed here so the number stays honest:

```
linkimport -> ai/enrich          linkimport -> search/search
telegram   -> platform/llm       telegram   -> platform/llmschema
```

The two `platform` edges are transport and cost nothing — a separate binary links the same
client. The two from `linkimport` are the real seam: importing a job by URL enriches it and
pushes it to the index inline.

The fifth is a different kind of edge, not part of that crawl-extraction accounting:

```
employer -> identity/accounts
```

Claiming a company reuses `accounts.Service`'s generic, purpose-keyed mailed-code
machinery (`IssueCode`/`ConfirmCode` — see `identity/accounts/AGENTS.md`) rather than
building a second one, so a company-account claim shares the same rate-limiting and
attempt-bounding every other account code already has. No other `ingest` package imports
`identity` today.
