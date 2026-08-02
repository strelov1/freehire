# Telegram Ukrainian intake — design

**Date:** 2026-08-01
**Status:** approved, ready for planning

## Problem

The Telegram crawl covers 88 channels, all Russian- or English-language. The Ukrainian IT
segment is absent from the catalogue, and the reason is not that no channels exist — it is
that the extraction queue's prefilter cannot recognise a Ukrainian vacancy.

`internal/telegram/prefilter.go` carries hiring markers in RU and EN only. Ukrainian
"вакансія" does not match the RU alternative `ваканси`: Cyrillic `і` (U+0456) and `и`
(U+0438) are distinct runes. A post that says «Шукаємо Golang розробника» reaches the
prefilter, fails it, is stored as done with zero vacancies, and never reaches the LLM.

Measured against live channel previews on 2026-08-01 (20 newest posts each, the `t.me/s`
web preview):

| Channel | posts | pass the current prefilter | Ukrainian markers would add |
|---|---|---|---|
| wwjobs | 20 | 4 | +15 |
| naymarnya | 18 | 3 | +7 |
| junior_dou_ua | 20 | 4 | +5 |
| halepnyirecruiting | 18 | 0 | +3 |

The same measurement on eight channels already in `sources/telegram.yml`
(`it_vakansii_jobs`, `vakansii_it`, `jobforjunior`, `huntmejob`, `normrabota`, `zrabota`,
`budujobs`, `product_jobs`) gives 17–19 of 20 passing and **zero** rescued by Ukrainian
markers. Nothing is leaking in production today — the gap only opens when Ukrainian channels
are onboarded, which is exactly what this change does.

A second gap sits downstream. `Львів`, `Харків`, `Одеса`, `Дніпро` resolve to a city name but
carry no country and no region, while `Berlin`, `Warsaw`, `Kraków` carry both. This is not a
bug in the parser: `internal/location/location.go:78-82` documents the contract that the
generated GeoNames `cityDict` supplies the canonical *name* only, never a country, so an
ambiguous city can never guess a geography. Country comes from the curated map in
`dictionaries.go`, which holds `kyiv`/`киев`/`київ` and no other Ukrainian entry. Without
this, a Lviv vacancy lands in the catalogue invisible to `country=ua` and to the `eu` region.

## Goal

Make the Ukrainian-language Telegram segment visible to the catalogue: recognised by the
prefilter, placed on the map, and represented by a vetted set of channels.

## Non-goals

- **Telegram group chats.** The source list carries 97 of them. `cmd/tg-ingest` reads the
  public `t.me/s` web preview, which serves posts for channels and nothing for groups —
  verified: `t.me/s/aiogramua`, `t.me/s/angular_community_ua`, `t.me/s/angularkyiv` all
  return HTTP 200 with zero message nodes. Reading them needs an MTProto client with a
  logged-in account, a different class of infrastructure.
- **`jobs.dou.ua`.** Returns HTTP 403 to a plain request; onboarding it is an anti-bot
  problem, not a source-file problem.
- **`wwjobs`.** Highest vacancy density of any candidate (81 keyword hits across 20 posts)
  but its newest post is 2026-06-10, 52 days old. `sources/telegram.yml` states its own
  admission gate — public preview plus a post within the last 30 days — and this channel
  fails it. Backlog item: revisit if it revives.
- **Telegram vacancy expiry.** Telegram jobs are reached by none of the three lifecycle
  close mechanisms and never close. That is catalogue-wide (all 88 channels), independent of
  this change, and gets its own spec. See Follow-up.
- No schema change, no migration.

## Decisions

### 1. Extend the existing prefilter regexp rather than restructuring it

The prefilter is one regexp organised into commented per-language blocks. Ukrainian becomes
a fourth block; `грн|₴` joins the currency alternation.

```go
// UA hiring verbs/nouns. Ukrainian "вакансія" does NOT match the RU "ваканси"
// above — the Cyrillic і and и are distinct runes.
`вакансі|шукаємо|шукає|запрошуємо|стажуванн|досвід роботи|` +
```

Rejected: splitting into named per-language regexps combined at init. Three languages do not
justify four variables, and one regexp compiles to one automaton scanned once per post,
where four mean up to four passes on the crawler's hot path. Rejected: language detection
before filtering — a detector is its own error source and the posts are routinely bilingual
(«Вакансія: Senior Go Developer, requirements below»).

### 2. The marker set is the measured one, not the plausible one

Each candidate was scored over 156 posts from eight job channels and 150 posts from eight
news channels, counting only posts the current prefilter rejects:

| marker | rescued in job channels | let through in news channels |
|---|---|---|
| `вакансі` | 22 | 0 |
| `досвід роботи` | 10 | 1 |
| `шукаємо\|шукає` | 8 | 1 |
| `грн`/`₴` amounts | 8 | 2 |
| `стажуванн` | 5 | 1 |
| `запрошуємо` | 4 | 0 |
| `потрібен\|потрібна\|потрібні` | 1 | 1 |
| `відгук` | 2 | 1 |
| `наймаємо` | 0 | 0 |
| `зарплатн` | 0 | 0 |

Adopted: the first six. Dropped: `наймаємо` (never fires), `зарплатн` (never fires — the RU
marker `зарплат` is already its prefix), `потрібен` and `відгук` (1:1 signal to noise).

The asymmetry that settles the borderline cases: `internal/telegram/AGENTS.md` states the LLM
is the real classifier. A false positive costs one LLM call returning `{"jobs": []}`. A false
negative loses a vacancy permanently and silently. Recall is worth more than precision here.

### 3. Ukrainian geography goes into the curated map, not the generated one

Two additions to `nameToCountry` in `internal/location/dictionaries.go`, in the blocks that
already hold `kyiv` and `київ`:

- Latin, beside line 98: `lviv`, `kharkiv`, `odesa`, `odessa`, `dnipro`, `vinnytsia`,
  `ivano-frankivsk`, `zaporizhzhia`, `chernivtsi`, `ternopil`, `uzhhorod`, `rivne`, `lutsk`,
  `poltava`, `khmelnytskyi`, `zhytomyr`, `cherkasy`, `sumy`, `mykolaiv`, `chernihiv`,
  `kryvyi rih` → `ua`.
- Cyrillic, beside line 146: the same cities in Ukrainian and Russian spelling, plus
  `україна`/`украина` → `ua`.

This respects the dict-only contract: country still comes only from the curated map, and
GeoNames still supplies nothing but the display name. With both sides agreeing on `ua`, the
agreement branch at `location.go:88` emits city *and* country.

### 4. Seven channels, two tiers

Verified 2026-08-01 — public `t.me/s` preview enabled and a post within the last 30 days.

| Channel | kind | newest post |
|---|---|---|
| `naymarnya` | board | 2026-07-10 |
| `halepnyirecruiting` | board | 2026-07-30 |
| `devops_dou` | authored | 2026-08-01 |
| `dou_qa` | authored | 2026-08-01 |
| `frontend_dou` | authored | 2026-08-01 |
| `gamedev_dou` | authored | 2026-08-01 |
| `junior_dou_ua` | authored | 2026-08-01 |

The DOU verticals are `authored`, not `board`: they publish digests where one post can carry
several roles, which is precisely what `KindAuthored` instructs the model to split
(`internal/telegram/llm.go:104`).

Source: [nikit0ns/Ukrainian_IT_Communities](https://github.com/nikit0ns/Ukrainian_IT_Communities),
192 entries — 97 group chats, 68 channels, 12 websites, 15 other platforms. Of the 68 channel
entries, 55 carry a public username (the rest are `t.me/+…` invite links, unreachable without
joining). Exactly one of those 55, `job_it_junior`, was already in `sources/telegram.yml`, so
the list is genuinely new material. Of the 54 new ones, about 32 posted within the last 30
days, 8 carry vacancies, and 7 also clear the freshness gate.

## Changes

| File | Change |
|---|---|
| `internal/telegram/prefilter.go` | UA marker block; `грн\|₴` in the currency alternation |
| `internal/telegram/prefilter_test.go` | UA pass and reject cases |
| `internal/location/dictionaries.go` | Ukrainian cities and country, Latin + Cyrillic |
| `internal/location/location_test.go` | `Львів`, `Lviv, Ukraine` → `ua` / `eu` / `Lviv` |
| `sources/telegram.yml` | 7 channels under two commented headings |
| `docs/telegram-channels.md` | new dated section; header count 88 → 95 (17 authored, 78 board) |

`docs/telegram-channels.md` declares itself a mirror of `sources/telegram.yml`. It is in sync
today (88 table rows against 88 YAML entries) and must stay so in the same commit.

## Testing

- `prefilter_test.go` gains UA rows in both existing tables. Pass: «Вакансія: Golang
  розробник», «Шукаємо QA у продуктову команду», «Досвід роботи від 2 років, зарплата 60 000
  грн». Reject: «Дайджест новин тижня», «Знижка на курс — встигни записатись».
- `location_test.go` gains cases mirroring the existing `Київ` one: `Львів` and
  `Lviv, Ukraine` → `Geo{Countries: ["ua"], Regions: ["eu"], Cities: ["Lviv"]}`.
- `go test ./...` and `go test -tags=integration ./internal/db/` both before push.
- Before/after measurement on live previews: `naymarnya` rises from 3/18, `halepnyirecruiting`
  from 0/18, and the eight RU channels already in production show **no change**. Extending an
  alternation can only add matches, so the only way to do harm is to start admitting noise in
  channels that already work — this measurement is what catches that.

## Follow-up

Telegram vacancy expiry, as its own change:

- Telegram jobs never close. The ingest sweep skips them (`telegram` is not a board provider;
  `cmd/tg-extract` writes through `UpsertJob` directly), there is no change feed to self-close
  from, and the liveness probe excludes them by name (`cmd/liveness/main.go:55`) because the
  stored URL is the post, which outlives the vacancy.
- `openspec/specs/job-lifecycle/spec.md:97` still requires the opposite — that a
  `source = 'telegram'` job be probed. The code comment cites a "job-lifecycle spec's telegram
  limitation" that exists in neither `docs/agents/job-lifecycle.md` nor openspec. Behaviour
  changed and the spec did not follow; that reconciliation belongs to the same change.
- Open question for that design: `docs/agents/job-lifecycle.md:8` fixes `closed_at` to mean
  "the employer took this down", and the project has already once declined to overload it
  (catalogue pruning got `pruned_jobs` instead). An age-based expiry is a third meaning —
  "presumed stale". Whether it writes `closed_at` anyway needs a deliberate answer.
- `wwjobs` — revisit if it starts posting again.
