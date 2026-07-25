<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <img src="docs/assets/logo-light.svg" alt="freehire" width="84" height="84">
</picture>

# freehire

### Every IT job, straight from the source.

**3.4M+ live postings pulled directly from company career pages — no recruiters, no reposts, no dead links. Fully open source.**

[**Try it live →**](https://freehire.me) · [Sources](#sources) · [API](#api) · [Add a source](#adding-a-source) · [Contributing](CONTRIBUTING.md)

[![Live](https://img.shields.io/badge/live-freehire.me-0a0a0a)](https://freehire.me)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go version](https://img.shields.io/github/go-mod/go-version/strelov1/freehire)
![Last commit](https://img.shields.io/github/last-commit/strelov1/freehire)
[![Stars](https://img.shields.io/github/stars/strelov1/freehire?style=social)](https://github.com/strelov1/freehire/stargazers)

<br>

<img src="docs/assets/freehire.gif" alt="freehire — faceted search narrowing 3.4M+ live postings by region, work format, specialization and seniority, each linking straight to the company's own careers page" width="860">

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

Aggregating **3.4M+ live postings** from **185,000+ companies** across **75+ ATS
platforms** and a long tail of aggregators and direct feeds — see
[Sources](#sources) for the full breakdown.

> If freehire saves you time — or you just like the idea of jobs straight from the
> source — a ⭐ helps other people find it.

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
| GET    | `/api/v1/me/tracking`             | ✓    | The caller's tracked/saved jobs          |
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

Live catalogue snapshot — **3,407,508 open postings** across **187,542
companies** (5,825,773 total incl. closed). Counts are open postings unless
noted. Every source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (mycareersfuture, himalayas, jobtech, Telegram…).
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).

### ATS platforms

**78 platforms · 80,370 companies · 2,901,510 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| workday | 4,047 | 831,217 |
| oracle | 526 | 291,963 |
| smartrecruiters | 2,748 | 257,443 |
| ukg | 1 | 206,768 |
| greenhouse | 6,782 | 178,084 |
| icims | 3,842 | 122,532 |
| paycom | 5,908 | 109,503 |
| jibe | 13 | 94,291 |
| apploi | 2,957 | 88,710 |
| gupy | 1,428 | 69,088 |
| bamboohr | 9,096 | 61,956 |
| lever | 2,126 | 56,453 |
| ashby | 3,580 | 55,136 |
| jazzhr | 3,789 | 46,179 |
| recruitee | 1,796 | 38,917 |
| phenom | 47 | 38,107 |
| personio | 3,992 | 36,097 |
| paylocity | 2,663 | 34,067 |
| hireology | 2,474 | 27,095 |
| applicantpro | 1,931 | 24,140 |
| eightfold | 41 | 21,098 |
| teamtailor | 1,354 | 19,868 |
| careerplug | 5,464 | 18,079 |
| isolvedhire | 2,166 | 18,063 |
| workable | 681 | 18,013 |
| zohorecruit | 1,066 | 17,734 |
| pinpoint | 660 | 14,994 |
| solides | 1,134 | 14,931 |
| breezy | 842 | 13,791 |
| join | 4,014 | 10,088 |
| jobylon | 841 | 8,367 |
| inhire | 363 | 8,078 |
| taleo | 13 | 7,623 |
| trakstar | 510 | 7,097 |
| freshteam | 147 | 4,761 |
| factorial | 460 | 4,484 |
| successfactors | 9 | 3,258 |
| erecruiter | 30 | 2,560 |
| gem | 217 | 2,456 |
| senior | 81 | 2,448 |
| traffit | 44 | 2,002 |
| cornerstone | 14 | 1,951 |
| jobvite | 54 | 1,766 |
| neogov | 11 | 1,497 |
| radancy | 5 | 1,445 |
| rippling | 77 | 1,158 |
| manatal | 13 | 1,032 |
| avature | 2 | 791 |
| loxo | 12 | 704 |
| peopleforce | 53 | 629 |
| deel | 58 | 541 |
| wpyoast | 1 | 402 |
| comeet | 17 | 389 |
| clinch | 1 | 387 |
| crelate | 55 | 154 |
| catsone | 4 | 149 |
| ashbygraphql | 3 | 125 |
| huntflow | 19 | 114 |
| ismartrecruit | 2 | 108 |
| pageup | 2 | 107 |
| jobscore | 6 | 89 |
| cleverstaff | 32 | 78 |
| bullhorn | 2 | 70 |
| careerspage | 3 | 43 |
| hurma | 5 | 42 |
| recruitingsolutions | 17 | 40 |
| earcu | 1 | 36 |
| quickin | 3 | 33 |
| talentlyft | 3 | 19 |
| adp | 1 | 19 |
| mindsight | 1 | 13 |
| vouch | 1 | 11 |
| odoo | 1 | 11 |
| enlizt | 1 | 5 |
| weblink | 4 | 4 |
| spark | 1 | 4 |
| talentadore | 1 | 3 |
| briefhq | 1 | 2 |

### Aggregators & job boards

**44 sources · 115,253 companies · 479,504 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| trudvsem | 37,381 | 213,394 |
| mycareersfuture | 19,791 | 70,428 |
| jobtech | 6,035 | 24,228 |
| gulftalent | 798 | 19,283 |
| himalayas | 8,418 | 17,965 |
| infojobs | 11,343 | 17,371 |
| jobnet | 5,325 | 11,946 |
| usajobs | 350 | 10,469 |
| tyomarkkinatori | 2,836 | 10,441 |
| jobdanmark | 3,453 | 9,885 |
| reed | 1,376 | 8,702 |
| justjoin | 1,040 | 8,644 |
| nofluffjobs | 334 | 8,435 |
| telegram | 2,871 | 7,795 |
| hh | 2,992 | 6,548 |
| djinni | 1,826 | 5,981 |
| wantedkr | 2,040 | 5,549 |
| jobstash | 634 | 4,166 |
| workatastartup | 1,310 | 4,020 |
| arbeitsagentur | 873 | 2,376 |
| vagas | 391 | 1,923 |
| arbeitnow | 1,299 | 1,883 |
| likeit | 16 | 1,254 |
| getonbrd | 279 | 1,066 |
| habr_career | 179 | 1,049 |
| functionalworks | 335 | 886 |
| thehub | 270 | 838 |
| getmatch | 128 | 701 |
| remoteok | 454 | 537 |
| getro | 112 | 419 |
| jobicy | 214 | 296 |
| geekjob | 158 | 246 |
| weworkremotely | 150 | 177 |
| wantapply | 54 | 159 |
| workablemarketplace | 2 | 140 |
| startupandvc | 74 | 100 |
| tecla | 31 | 53 |
| remotive | 21 | 42 |
| workingnomads | 16 | 39 |
| getmanfred | 26 | 34 |
| jobspresso | 13 | 20 |
| topco | 4 | 8 |
| teamex | 1 | 8 |
| 4dayweek | 0 | 0 |

### Company career sites

**34 feeds · 64 companies · 26,488 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| amazon | 1 | 8,039 |
| apple | 1 | 4,282 |
| google | 7 | 3,484 |
| alfabank | 1 | 2,133 |
| sber | 10 | 1,801 |
| mts | 12 | 1,217 |
| epam | 1 | 978 |
| yandex | 1 | 859 |
| luxoft | 1 | 739 |
| uber | 1 | 562 |
| tbank | 1 | 456 |
| rwb | 1 | 396 |
| micro1 | 1 | 289 |
| vk | 1 | 279 |
| bairesdev | 1 | 171 |
| avito | 1 | 148 |
| lamoda | 1 | 134 |
| dataart | 1 | 133 |
| alignerr | 1 | 59 |
| globalpayments | 1 | 58 |
| meta | 1 | 43 |
| vention | 1 | 32 |
| aviasales | 1 | 30 |
| northstone | 3 | 27 |
| domclick | 1 | 27 |
| rapyd | 1 | 23 |
| ozon | 1 | 21 |
| 2gis | 1 | 15 |
| lumenalta | 1 | 15 |
| dodo | 3 | 12 |
| onstrider | 1 | 11 |
| mtslink | 1 | 7 |
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

freehire's core is a small pipeline; the easiest way to help is to **add a
source** — one entry in a `sources/` board file, or a new adapter in
`internal/sources`. Questions and ideas are always welcome in
[Discussions](https://github.com/strelov1/freehire/discussions). Ready to send a
change? **Open an issue first** — it gets you on the contributor allowlist and
points you at the right seam. See [CONTRIBUTING.md](CONTRIBUTING.md) for the
workflow and [AGENTS.md](AGENTS.md) for the architecture and conventions. (Issues
and PRs from accounts not yet on the allowlist are auto-closed to keep out spam —
a quick intro issue is all it takes.)

## Security

Found a vulnerability? Report it privately — see [SECURITY.md](SECURITY.md). Do
not open a public issue for security-sensitive reports.

## License

[MIT](LICENSE)
