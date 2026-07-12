<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <img src="docs/assets/logo-light.svg" alt="freehire" width="84" height="84">
</picture>

# freehire

### Every IT job, straight from the source.

**3M+ live postings pulled directly from company career pages — no recruiters, no reposts, no dead links. Fully open source.**

[**Try it live →**](https://freehire.dev) · [Sources](#sources) · [API](#api) · [Add a source](#adding-a-source) · [Contributing](CONTRIBUTING.md)

[![Live](https://img.shields.io/badge/live-freehire.dev-0a0a0a)](https://freehire.dev)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go version](https://img.shields.io/github/go-mod/go-version/strelov1/freehire)
![Last commit](https://img.shields.io/github/last-commit/strelov1/freehire)
[![Stars](https://img.shields.io/github/stars/strelov1/freehire?style=social)](https://github.com/strelov1/freehire/stargazers)

<br>

<img src="docs/assets/screenshot.png" alt="freehire — the job feed with faceted filters for region, work format, specialization and seniority" width="860">

</div>

## Why freehire?

- **Straight from the source.** Every listing is crawled directly from a company's
  own ATS — Workday, Greenhouse, Lever, Ashby, iCIMS and a long tail of others — and
  links to the original posting. No recruiter reposts, no aggregator middlemen, no
  dead links.
- **One schema, deduplicated.** The same role posted to three boards collapses into
  one entry: every posting is normalized into a single shape and deduplicated on a
  stable key.
- **Search that understands jobs.** Faceted full-text search over region, work mode,
  seniority, skills and salary — derived from curated dictionaries, never guessed.
- **Actually open.** MIT-licensed and self-hostable, pipeline and data both in the
  open. Adding a company is one line of YAML.
- **Yours to build on.** A clean HTTP API, a CLI, Telegram digests, and per-user
  application tracking — use the hosted site, run your own, or build on top.

Aggregating **3M+ live postings** from **115,000+ companies** across **55+ ATS
platforms** and a long tail of aggregators and direct feeds — see
[Sources](#sources) for the full breakdown.

## Stack

- **Go** + [Fiber v2](https://gofiber.io/) — HTTP server
- **PostgreSQL** — storage and filtering
- **[sqlc](https://sqlc.dev/)** — type-safe DB access from SQL (no ORM)
- **[Meilisearch](https://www.meilisearch.com/)** — full-text and faceted job search
- **[langchaingo](https://github.com/tmc/langchaingo)** — LLM access over any OpenAI-compatible endpoint (no vendor baked in)
- **Docker Compose** — local development

## Quick start

```bash
make up        # build + start app, postgres, and meilisearch in Docker
curl localhost:8080/health
curl localhost:8080/api/v1/jobs
```

Migrations are applied automatically on first Postgres volume init
(the `migrations/` folder is mounted into `/docker-entrypoint-initdb.d`).
Changing a migration does not re-apply to an existing volume — recreate it with
`docker compose down -v && make up`, or apply pending files manually with
`make migrate`.

If port 8080 is already taken, pick another host port:

```bash
HIRE_HOST_PORT=8090 make up
```

## Local development

```bash
docker compose up -d db   # database only
make run                  # server on host, reads DATABASE_URL
```

Copy `.env.example` to `.env` and adjust as needed. `JWT_SECRET` is required for
the server to start; OAuth and LLM credentials are optional (the features they
gate stay disabled when unset).

## Commands

```bash
make help      # list all commands
make sqlc      # regenerate code from SQL (via Docker, no local sqlc needed)
make tidy      # go mod tidy
make psql      # psql inside the DB container
make reindex   # rebuild the Meilisearch index from Postgres
make migrate   # apply migrations manually to an existing DB volume
```

## Workers

The server only serves the API. Ingest and enrichment are standalone, run-once
workers meant for cron — each crawls or drains its queue and exits.

```bash
go run ./cmd/ingest sources/greenhouse.yml  # crawl one board file and upsert jobs (path also via SOURCES_FILE)
go run ./cmd/enrich        # drain the enrichment queue (LLM); needs LLM_* config
go run ./cmd/tg-ingest     # crawl the Telegram channels in sources/telegram.yml
go run ./cmd/tg-extract    # LLM-extract vacancies from crawled Telegram posts
go run ./cmd/reindex       # rebuild the Meilisearch index from Postgres
go run ./cmd/backfill-derive  # re-derive all six dictionary facets on existing jobs (follow with make reindex)
```

## Layout

```
cmd/                 entry points: server + the standalone workers above
sources/             board files, one per provider (e.g. greenhouse.yml = company + board id),
                     plus a mixed custom.yml and telegram.yml (Telegram channels to crawl)
internal/
  config/            env configuration
  database/          pgxpool connection pool
  db/                generated sqlc code + queries/*.sql
  handler/           HTTP handlers
  auth/              auth primitives (JWT cookie, API keys) + OAuth sign-in
  sources/           ATS source adapters (greenhouse / lever / ashby) + registry
  linksource/        resolves outbound job links found in Telegram posts
  telegram/          Telegram-channel crawl + LLM vacancy extraction
  pipeline/          ingest runner (fetch → normalize → dedup → upsert)
  enrich/            typed AI-enrichment contract + queue-draining runner
  search/            Meilisearch indexing and query
  location/          geography parsed from free-text ATS location strings
  jobview/           the single public wire shape of a job
  normalize/         slug normalization
migrations/          SQL schema (source for both sqlc and initdb)
```

## API

All responses use `{"data": ...}` (single), `{"data": ..., "meta": {...}}`
(lists), or `{"error": msg}`. Jobs and companies are addressed by their public
slug.

| Method | Path                              | Auth | Description                              |
|--------|-----------------------------------|------|------------------------------------------|
| GET    | `/health`                         | —    | Service and DB status                    |
| GET    | `/api/v1/jobs`                    | —    | List jobs (`limit`/`offset`)             |
| GET    | `/api/v1/jobs/search`             | —    | Full-text + faceted search               |
| GET    | `/api/v1/jobs/:slug`              | —    | Job by slug                              |
| GET    | `/api/v1/companies`               | —    | List companies                           |
| GET    | `/api/v1/companies/:slug`         | —    | Company by slug                          |
| POST   | `/api/v1/jobs/:slug/view`         | ✓    | Record a view                            |
| POST   | `/api/v1/jobs/:slug/apply`        | ✓    | Mark applied                             |
| POST   | `/api/v1/jobs/:slug/save`         | ✓    | Save (bookmark)                          |
| DELETE | `/api/v1/jobs/:slug/save`         | ✓    | Unsave                                   |
| PATCH  | `/api/v1/jobs/:slug/track`        | ✓    | Set application stage / notes            |
| GET    | `/api/v1/me/jobs`                 | ✓    | The caller's tracked/saved jobs          |
| POST   | `/api/v1/me/api-keys`             | 🍪   | Create an API key (returns it once)      |
| GET    | `/api/v1/me/api-keys`             | 🍪   | List API keys                            |
| DELETE | `/api/v1/me/api-keys/:id`         | 🍪   | Revoke an API key                        |
| POST   | `/api/v1/auth/register`           | —    | Register                                 |
| POST   | `/api/v1/auth/login`              | —    | Log in                                   |
| POST   | `/api/v1/auth/logout`             | —    | Log out                                  |
| GET    | `/api/v1/auth/me`                 | ✓    | The current user                         |
| GET    | `/api/v1/auth/oauth/providers`    | —    | Enabled OAuth providers                  |
| GET    | `/api/v1/auth/oauth/:p/start`     | —    | Begin OAuth sign-in                      |
| GET    | `/api/v1/auth/oauth/:p/callback`  | —    | OAuth callback (sets the session cookie) |

Auth legend: **✓** session cookie or API key · **🍪** session cookie only.

## Sources

Live catalogue snapshot — **3,014,475 open postings** across **119,822
companies** (4,640,025 total incl. closed). Counts are open postings unless
noted. Every source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (mycareersfuture, himalayas, jobtech, Telegram…).
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).

### ATS platforms

**58 platforms · 76,768 companies · 2,745,916 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| oracle | 526 | 388,623 |
| workday | 4,043 | 378,190 |
| smartrecruiters | 2,712 | 315,060 |
| greenhouse | 6,767 | 202,938 |
| ukg | 1 | 191,410 |
| icims | 3,766 | 165,556 |
| paycom | 5,906 | 135,880 |
| jibe | 13 | 112,388 |
| apploi | 2,957 | 96,540 |
| gupy | 1,427 | 78,310 |
| lever | 2,112 | 71,928 |
| bamboohr | 9,055 | 64,423 |
| ashby | 3,563 | 58,268 |
| jazzhr | 3,775 | 53,764 |
| phenom | 46 | 51,194 |
| recruitee | 1,735 | 41,270 |
| personio | 3,989 | 37,746 |
| paylocity | 2,536 | 32,485 |
| hireology | 2,475 | 29,588 |
| applicantpro | 1,923 | 21,044 |
| isolvedhire | 2,162 | 20,316 |
| zohorecruit | 1,064 | 18,921 |
| teamtailor | 1,161 | 18,383 |
| careerplug | 4,749 | 18,079 |
| pinpoint | 608 | 18,077 |
| eightfold | 37 | 16,567 |
| workable | 550 | 15,846 |
| solides | 1,132 | 15,681 |
| breezy | 780 | 14,558 |
| join | 4,004 | 11,921 |
| inhire | 350 | 8,229 |
| taleo | 13 | 8,200 |
| trakstar | 508 | 7,093 |
| freshteam | 147 | 4,732 |
| factorial | 460 | 4,644 |
| senior | 80 | 2,677 |
| erecruiter | 30 | 2,609 |
| gem | 217 | 2,596 |
| cornerstone | 13 | 1,893 |
| radancy | 5 | 1,552 |
| neogov | 11 | 1,367 |
| successfactors | 4 | 875 |
| rippling | 75 | 758 |
| loxo | 12 | 706 |
| deel | 58 | 600 |
| comeet | 17 | 400 |
| traffit | 14 | 396 |
| wpyoast | 1 | 391 |
| clinch | 1 | 382 |
| avature | 1 | 363 |
| ashbygraphql | 3 | 129 |
| huntflow | 18 | 107 |
| pageup | 2 | 102 |
| careerspage | 1 | 48 |
| earcu | 1 | 43 |
| recruitingsolutions | 17 | 40 |
| adp | 1 | 19 |
| vouch | 1 | 11 |

### Aggregators & job boards

**27 sources · 47,355 companies · 234,947 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| mycareersfuture | 17,849 | 91,840 |
| gulftalent | 807 | 23,667 |
| jobtech | 4,509 | 21,636 |
| himalayas | 7,713 | 15,955 |
| jobdanmark | 4,773 | 15,188 |
| jobnet | 4,652 | 11,516 |
| usajobs | 330 | 11,029 |
| justjoin | 997 | 10,983 |
| telegram | 2,527 | 6,642 |
| reed | 833 | 5,587 |
| wantedkr | 1,742 | 5,333 |
| workatastartup | 1,383 | 5,058 |
| jobstash | 529 | 3,489 |
| arbeitnow | 1,031 | 1,765 |
| habr_career | 205 | 1,155 |
| thehub | 264 | 1,101 |
| getonbrd | 257 | 1,052 |
| getmatch | 119 | 749 |
| jobicy | 249 | 407 |
| remoteok | 288 | 342 |
| geekjob | 113 | 161 |
| weworkremotely | 120 | 138 |
| tecla | 29 | 51 |
| remotive | 20 | 44 |
| workingnomads | 15 | 41 |
| topco | 4 | 10 |
| teamex | 1 | 8 |

### Company career sites

**28 feeds · 56 companies · 33,606 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| amazon | 1 | 10,287 |
| apple | 1 | 4,861 |
| google | 7 | 3,506 |
| sber | 10 | 2,641 |
| mts | 12 | 2,447 |
| alfabank | 1 | 2,341 |
| tbank | 1 | 1,997 |
| luxoft | 1 | 1,389 |
| yandex | 1 | 859 |
| epam | 1 | 681 |
| meta | 1 | 630 |
| uber | 1 | 604 |
| rwb | 1 | 388 |
| vk | 1 | 280 |
| avito | 1 | 164 |
| lamoda | 1 | 147 |
| dataart | 1 | 144 |
| globalpayments | 1 | 58 |
| vention | 1 | 37 |
| aviasales | 1 | 33 |
| domclick | 1 | 24 |
| rapyd | 1 | 22 |
| ozon | 1 | 20 |
| lumenalta | 1 | 17 |
| dodo | 3 | 11 |
| mtslink | 1 | 10 |
| telegramcareers | 1 | 6 |
| kuper | 1 | 2 |

Plus **6** postings from manual bulk imports.

## Adding a source

Adding a company is one entry in the provider's board file (`sources/<provider>.yml`,
or the mixed `sources/custom.yml`) — `company` + `board` (and `provider` when an
entry overrides the file's). Adding an ATS platform is a new adapter in
`internal/sources` plus one line in `sources.All` — every adapter speaks the same
`Source` interface, and `cmd/ingest` validates the file against the registry before
any crawl.

For most companies the platform is already supported, so adding them is just one
line in the board file. Only when a company runs on an ATS we don't cover yet does
it need a new provider (a new adapter). Either way, if you want a source added,
**start by [opening an issue](https://github.com/strelov1/freehire/issues)** — name
the company and its careers URL, and we'll confirm whether it's a one-line add or a
new provider before any code.

## Frontend

A Svelte SPA lives under `web/` and consumes the API (same-origin; a dev Vite
proxy forwards `/api` to the backend).

## Contributing

freehire's core is a small pipeline; the extension point is the **source**
(one entry in a `sources/` board file, or a new adapter in `internal/sources`). New
contributors: open an issue first — issues and PRs from unapproved accounts are
auto-closed by default. See [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow
and [AGENT.md](AGENT.md) for the architecture and conventions. Questions and
ideas go in [Discussions](https://github.com/strelov1/freehire/discussions).

## Security

Found a vulnerability? Report it privately — see [SECURITY.md](SECURITY.md). Do
not open a public issue for security-sensitive reports.

## License

[MIT](LICENSE)
