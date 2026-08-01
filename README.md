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

Live catalogue snapshot — **3,458,772 open postings** across **222,652 companies**.
Counts are open postings unless noted; a company crawled from two sources is
counted under each. Every source is one of three kinds:

- **ATS platforms** — one adapter per multi-tenant applicant-tracking system,
  each serving many companies (Workday, Greenhouse, Lever, iCIMS…).
- **Aggregators & job boards** — third-party feeds that republish many
  companies' postings (mycareersfuture, himalayas, jobtech, Telegram…).
- **Company career sites** — direct single-company feeds crawled from a
  company's own careers page (Amazon, Apple, Google, Yandex, Sber…).

### ATS platforms

**83 platforms · 76,775 companies · 2,795,749 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| workday | 4,091 | 808,159 |
| oracle | 526 | 394,586 |
| smartrecruiters | 2,133 | 266,328 |
| greenhouse | 6,649 | 173,343 |
| ukg | 1 | 150,328 |
| icims | 2,967 | 106,481 |
| paycom | 5,737 | 93,330 |
| gupy | 1,428 | 71,908 |
| lever | 2,133 | 63,579 |
| apploi | 2,957 | 62,286 |
| ashby | 3,658 | 58,981 |
| bamboohr | 8,813 | 53,975 |
| jazzhr | 3,731 | 44,772 |
| phenom | 47 | 43,029 |
| recruitee | 1,829 | 41,280 |
| personio | 4,001 | 37,802 |
| paylocity | 2,619 | 26,115 |
| eightfold | 43 | 25,667 |
| jibe | 15 | 23,210 |
| teamtailor | 1,434 | 21,104 |
| applicantpro | 1,750 | 21,004 |
| zohorecruit | 1,073 | 20,482 |
| workable | 681 | 18,951 |
| hireology | 2,241 | 16,713 |
| pinpoint | 647 | 15,169 |
| isolvedhire | 1,984 | 14,657 |
| careerplug | 4,112 | 14,255 |
| breezy | 972 | 13,977 |
| solides | 1,124 | 13,689 |
| join | 3,940 | 9,780 |
| jobylon | 872 | 8,578 |
| inhire | 365 | 8,229 |
| taleo | 13 | 6,355 |
| trakstar | 501 | 6,183 |
| freshteam | 179 | 5,192 |
| successfactors | 12 | 4,691 |
| factorial | 476 | 4,563 |
| erecruiter | 30 | 2,901 |
| gem | 217 | 2,418 |
| senior | 82 | 2,309 |
| traffit | 44 | 2,127 |
| cornerstone | 14 | 1,720 |
| radancy | 5 | 1,648 |
| jobvite | 54 | 1,475 |
| avature | 3 | 1,331 |
| rippling | 86 | 1,300 |
| betterteam | 146 | 1,274 |
| neogov | 11 | 1,224 |
| pageup | 8 | 968 |
| manatal | 13 | 876 |
| softgarden | 18 | 825 |
| loxo | 12 | 677 |
| deel | 58 | 584 |
| peopleforce | 61 | 572 |
| comeet | 17 | 450 |
| clinch | 1 | 409 |
| wpyoast | 1 | 397 |
| opencats | 9 | 260 |
| compleo | 4 | 182 |
| catsone | 4 | 157 |
| ashbygraphql | 3 | 138 |
| huntflow | 19 | 129 |
| ismartrecruit | 2 | 103 |
| cleverstaff | 34 | 80 |
| jobscore | 6 | 79 |
| bullhorn | 2 | 75 |
| hurma | 5 | 46 |
| earcu | 1 | 44 |
| careerspage | 3 | 43 |
| recruitingsolutions | 17 | 40 |
| quickin | 3 | 34 |
| adp | 1 | 22 |
| talentlyft | 3 | 19 |
| odoo | 1 | 13 |
| speedrun | 10 | 12 |
| enlizt | 2 | 12 |
| mindsight | 1 | 12 |
| vouch | 1 | 10 |
| spark | 1 | 9 |
| weblink | 5 | 5 |
| talenthr | 1 | 4 |
| talentadore | 1 | 3 |
| briefhq | 1 | 2 |

### Aggregators & job boards

**45 sources · 147,231 companies · 534,902 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| trudvsem | 49,963 | 186,058 |
| mycareersfuture | 20,053 | 81,661 |
| jobtech | 8,557 | 28,713 |
| infojobs | 13,829 | 24,861 |
| himalayas | 12,173 | 23,368 |
| gulftalent | 776 | 19,828 |
| nofluffjobs | 404 | 19,655 |
| jobdanmark | 5,568 | 16,145 |
| jobnet | 6,407 | 14,214 |
| tyomarkkinatori | 3,371 | 11,959 |
| justjoin | 1,204 | 11,833 |
| reed | 1,500 | 11,738 |
| powertofly | 30 | 9,758 |
| telegram | 3,398 | 9,715 |
| usajobs | 360 | 9,045 |
| hh | 4,461 | 8,821 |
| jobstash | 930 | 7,536 |
| djinni | 2,140 | 7,065 |
| wantedkr | 2,368 | 5,976 |
| workatastartup | 1,424 | 5,195 |
| arbeitsagentur | 1,793 | 4,420 |
| arbeitnow | 2,074 | 3,614 |
| vagas | 455 | 1,903 |
| likeit | 16 | 1,469 |
| thehub | 307 | 1,272 |
| instaffo | 590 | 1,203 |
| getonbrd | 300 | 1,144 |
| habr_career | 186 | 1,144 |
| remoteok | 782 | 897 |
| functionalworks | 333 | 884 |
| getmatch | 143 | 838 |
| jobicy | 424 | 565 |
| wantapply | 100 | 522 |
| getro | 119 | 450 |
| weworkremotely | 314 | 432 |
| workablemarketplace | 2 | 391 |
| geekjob | 174 | 283 |
| startupandvc | 74 | 100 |
| tecla | 36 | 66 |
| workingnomads | 20 | 49 |
| remotive | 23 | 38 |
| getmanfred | 31 | 37 |
| jobspresso | 14 | 18 |
| topco | 4 | 11 |
| teamex | 1 | 8 |

### Company career sites

**35 feeds · 65 companies · 36,750 open postings.**

| Source | Companies | Open jobs |
| --- | ---: | ---: |
| amazon | 1 | 10,901 |
| apple | 1 | 4,804 |
| google | 7 | 3,781 |
| sber | 10 | 3,750 |
| tbank | 1 | 2,479 |
| alfabank | 1 | 2,385 |
| mts | 12 | 1,800 |
| luxoft | 1 | 1,334 |
| epam | 1 | 1,143 |
| yandex | 1 | 935 |
| meta | 1 | 760 |
| uber | 1 | 669 |
| rwb | 1 | 383 |
| micro1 | 1 | 322 |
| vk | 1 | 287 |
| bairesdev | 1 | 172 |
| avito | 1 | 153 |
| dataart | 1 | 143 |
| lamoda | 1 | 106 |
| vention | 1 | 66 |
| alignerr | 1 | 55 |
| globalpayments | 1 | 54 |
| northstone | 3 | 52 |
| aviasales | 1 | 32 |
| yandexcrowd | 1 | 31 |
| domclick | 1 | 27 |
| rapyd | 1 | 25 |
| ozon | 1 | 23 |
| dodo | 3 | 19 |
| 2gis | 1 | 18 |
| onstrider | 1 | 14 |
| lumenalta | 1 | 13 |
| mtslink | 1 | 7 |
| telegramcareers | 1 | 5 |
| kuper | 1 | 2 |

Plus **7** postings from manual bulk imports.

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
