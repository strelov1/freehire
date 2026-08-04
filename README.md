<div align="center">

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/assets/logo-dark.svg">
  <img src="docs/assets/logo-light.svg" alt="freehire" width="84" height="84">
</picture>

# freehire

### Every IT job, straight from the source.

**3.4M+ live postings pulled directly from company career pages — no recruiters, no reposts, no dead links. Fully open source.**

[**Try it live →**](https://freehire.me) · [Features](docs/features.md) · [Sources](#sources) · [API](#api) · [Add a source](#adding-a-source) · [Contributing](CONTRIBUTING.md)

[![Live](https://img.shields.io/badge/live-freehire.me-0a0a0a)](https://freehire.me)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
![Go version](https://img.shields.io/github/go-mod/go-version/strelov1/freehire)
![Last commit](https://img.shields.io/github/last-commit/strelov1/freehire)
[![Stars](https://img.shields.io/github/stars/strelov1/freehire?style=social)](https://github.com/strelov1/freehire/stargazers)
[![Discord](https://img.shields.io/badge/Discord-join-5865F2?logo=discord&logoColor=white)](https://discord.gg/aAXS2rghW)

<br>

<!-- Product Hunt: launching 26 August 2026. The badge is Product Hunt's own embed,
     one SVG per theme, picked by prefers-color-scheme the same way the logo above is.
     Its own utm_campaign so this traffic is separable from the site footer's badge. -->
<a href="https://www.producthunt.com/products/freehire?embed=true&amp;utm_source=badge-featured&amp;utm_medium=badge&amp;utm_campaign=badge-readme">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1196233&amp;theme=dark">
    <img src="https://api.producthunt.com/widgets/embed-image/v1/featured.svg?post_id=1196233&amp;theme=light" alt="freehire — every IT job, straight from the source | Product Hunt" width="250" height="54">
  </picture>
</a>

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
- **Yours to build on.** A clean HTTP API, a CLI, Telegram digests, and a whole
  workspace on top of the catalogue — see [Beyond the catalogue](#beyond-the-catalogue).
  Use the hosted site, run your own, or build on top.

Aggregating **3.4M+ live postings** from **220,000+ companies** across **80+ ATS
platforms** and a long tail of aggregators and direct feeds — see
[Sources](#sources) for the full breakdown.

> If freehire saves you time — or you just like the idea of jobs straight from the
> source — a ⭐ helps other people find it.

## Beyond the catalogue

Finding the posting is half the problem. The other half — writing the CV,
sending it, and knowing what happened to it — is built on the same data, in the
same repository, under the same licence.

| | |
| --- | --- |
| **Find** | Faceted search, curated collections, saved searches with email/Telegram digests, shared boards, market analytics, the ghost-job signal |
| **Apply** | CV builder with ATS-safe PDF templates, deterministic CV↔vacancy scoring, AI fit analysis, CV tailoring that invents nothing, tracer links, referrals |
| **Track** | An application board with stages, a mail inbox that links recruiter replies to the application they answer, an append-only event ledger, reminders |
| **Ask** | An in-process agent with five presets — chat, browse, profile, CV tailoring, interview rehearsal — with no shell and no minted credential |
| **Build on** | A keyless public API, a CLI, a form-filling browser extension, ChatGPT Actions |

**[Full feature reference → `docs/features.md`](docs/features.md)** — what each one
does, where it lives in the tree, which need an LLM endpoint configured, and
which draw on AI credits.

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
  cv/ cvedit/        structured CVs + PDF rendering; cvedit is their only writer
  experience/        durable employments + evidence atoms behind every CV claim
  userjob/           per-user job tracking (view / apply / save / stages)
  inbox/             recruiter mail → classify → link to an application
  assistant/         the in-process agent: turn loop, tools, transcripts
migrations/          SQL schema (source for both sqlc and initdb)
```

## API

The catalogue is served over a public, keyless HTTP API — `GET
https://freehire.me/api/v1/jobs` needs no credential. All responses use
`{"data": ...}` (single), `{"data": ..., "meta": {...}}` (lists), or
`{"error": msg}`; jobs and companies are addressed by their public slug.

**Full reference — every endpoint, its parameters, auth mode and the whole
search-filter vocabulary: [freehire.me/docs/api](https://freehire.me/docs/api).**

## Sources

Live catalogue snapshot — **3,445,246 open postings** across **223,685 companies**.
Counts are open postings unless noted; a company crawled from two sources is
counted under each. Every source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
  **84 platforms · 2,797,390 open postings.**
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (mycareersfuture, himalayas, jobtech, Telegram…).
  **47 sources · 612,690 open postings.**
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).
  **36 feeds · 35,160 open postings.**

Full per-source breakdown — every one of the 167 sources with its own
companies/open-jobs count — lives on its own page: [docs/sources.md](docs/sources.md).

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

**Contributions are welcome, and issues and PRs are open to everyone.** No
allowlist, no approval step — open one.

The easiest way to help is to **add a source**: one entry in a `sources/` board
file, or a new adapter in `internal/sources`. Missing a company you would apply
to? That is a one-line PR, and it is the single most useful thing you can send.

Questions and half-formed ideas are equally welcome in
[Discussions](https://github.com/strelov1/freehire/discussions) — no need to
polish them into an issue first. For anything large, an issue up front saves you
from building the wrong thing, but nothing stops you from just opening the PR.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the checks CI runs and
[AGENTS.md](AGENTS.md) for the architecture and conventions.

## Security

Found a vulnerability? Report it privately — see [SECURITY.md](SECURITY.md). Do
not open a public issue for security-sensitive reports.

## License

[MIT](LICENSE)
