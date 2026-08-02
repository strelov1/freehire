## Context

`cmd/tg-ingest` crawls each configured channel's public `t.me/s` preview and enqueues posts;
`cmd/tg-extract` drains that queue through an LLM. Between them sits `LooksLikeVacancy`
(`internal/telegram/prefilter.go`) — one regexp organised into commented per-language blocks
(RU, EN, plus salary-amount patterns). Its job is to spare the metered stage from posts that
are obviously not vacancies; `internal/telegram/AGENTS.md` states the LLM is the real
classifier.

The regexp has no Ukrainian block, and the Russian one does not reach Ukrainian by accident:
`ваканси` cannot match `вакансія` because `и` (U+0438) and `і` (U+0456) are distinct runes.

Measured 2026-08-01 against live previews (20 newest posts per channel):

| Channel | posts | pass today | Ukrainian markers add |
|---|---|---|---|
| wwjobs | 20 | 4 | +15 |
| naymarnya | 18 | 3 | +7 |
| junior_dou_ua | 20 | 4 | +5 |
| halepnyirecruiting | 18 | 0 | +3 |

The same measurement over eight channels already in `sources/telegram.yml` gives 17–19 of 20
passing and zero rescued — production is not leaking today. The gap opens only when Ukrainian
channels are onboarded.

Downstream, `internal/location` resolves an extracted `location` string. `Львів`, `Харків`,
`Одеса`, `Дніпро` yield a city name with no country and no region, while `Berlin`, `Warsaw`,
`Kraków` yield both. That is the documented contract at `internal/location/location.go:78-82`
working as designed: the generated GeoNames `cityDict` supplies the canonical name only, never
a country, so an ambiguous city can never guess a geography. Country comes from the curated
`nameToCountry` map, which holds `kyiv`/`киев`/`київ` and no other Ukrainian entry.

Source of candidates: [nikit0ns/Ukrainian_IT_Communities](https://github.com/nikit0ns/Ukrainian_IT_Communities),
192 entries — 97 group chats, 68 channels, 12 websites, 15 other platforms.

## Goals / Non-Goals

**Goals:**

- The prefilter recognises Ukrainian vacancy posts.
- Ukrainian cities resolve to `ua` and to the `eu` region, so the vacancies are reachable by
  the country and region filters.
- Seven vetted Ukrainian channels enter the crawl, and the human-readable mirror stays in sync.

**Non-Goals:**

- **Telegram group chats** (97 in the source list). `t.me/s` serves posts for channels and
  nothing for groups — verified on `aiogramua`, `angular_community_ua`, `angularkyiv`: HTTP
  200, zero message nodes. Reading them needs an MTProto client with a logged-in account.
- **`jobs.dou.ua`** — HTTP 403 to a plain request; an anti-bot problem, not a source-file one.
- **`wwjobs`** — the densest candidate (81 keyword hits across 20 posts) but its newest post
  is 2026-06-10, 52 days old, failing the 30-day admission gate `sources/telegram.yml` states
  for itself. Revisit if it revives.
- **Telegram vacancy expiry** — Telegram jobs are reached by none of the three lifecycle close
  mechanisms and never close. Catalogue-wide, independent of this change, own spec.
- No schema change, no migration, no new dependency.

## Decisions

### Extend the existing regexp rather than restructure it

Ukrainian becomes a fourth commented block; `грн|₴` joins the currency alternation.

*Alternative — named per-language regexps combined at init.* Rejected: three languages do not
justify four variables, and one regexp compiles to one automaton scanned once per post, where
four mean up to four passes on the crawler's hot path. The split would earn its keep if the
markers were config-driven; they are static.

*Alternative — detect the post's language, then apply a language-specific filter.* Rejected:
a detector is its own error source, and these posts are routinely bilingual («Вакансія: Senior
Go Developer, requirements below»).

### The marker set is the measured one

Each candidate scored over 156 posts from eight job channels and 150 from eight news channels,
counting only posts the current prefilter rejects:

| marker | rescued (job) | admitted (news) |
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

Adopted: the first six. Dropped: `наймаємо` and `зарплатн` (never fire — `зарплатн` is already
covered by the RU prefix `зарплат`), `потрібен` and `відгук` (1:1 signal to noise).

The shipped regexp writes `шукає` alone rather than `шукаємо|шукає`: the shorter stem is a
prefix of the longer, so it matches everything the pair did. Each adopted marker is the
shortest stem covering its inflections, for the same reason.

The asymmetry settling the borderline cases: a false positive costs one LLM call returning
`{"jobs": []}`; a false negative loses a vacancy permanently and silently.

### Ukrainian geography goes into the curated map, not the generated one

Two additions to `nameToCountry` in `internal/location/dictionaries.go`, in the blocks that
already hold `kyiv` and `київ`: the 21 regional capitals in Latin, and the same in Ukrainian
and Russian Cyrillic plus `україна`/`украина`.

This keeps the dict-only contract intact — country still comes only from the curated map, and
GeoNames still supplies nothing but a display name. With both sides agreeing on `ua`, the
agreement branch at `location.go:88` emits city *and* country.

*Alternative — attach the country from `cityDict`.* Rejected: it would let every ambiguous
city in the GeoNames table guess a geography, which is the exact failure the contract exists
to prevent.

### DOU verticals are `authored`, not `board`

They publish digests where one post can carry several roles, which is what `KindAuthored`
instructs the model to split (`internal/telegram/llm.go:104`). The two recruiter channels are
`board` — one post, one vacancy.

## Risks / Trade-offs

- **More posts reach the metered LLM stage.** → Bounded by seven channels; the adopted set's
  false-positive rate is 5 posts per 150 on channels carrying no vacancies at all.
- **A broader alternation could admit noise in channels that already work.** → Extending an
  alternation is monotonic: it can only add matches, so this is the only way the change can do
  harm. The before/after measurement on the eight production RU channels must show no change.
- **`досвід роботи` can appear in course advertising.** → 1 admission per 150 news posts,
  accepted given the recall asymmetry. If it degrades, it is one alternative to remove.
- **The docs mirror can drift from the YAML.** → Both edited in the same commit; the counts are
  in sync today (88 rows against 88 entries) and verified after the change (95).
- **Ukrainian city names are added by hand.** → Confined to the 21 regional capitals; long-tail
  towns keep resolving to a bare city name, the same as any other country's long tail.

## Migration Plan

No migration. The change is data plus one regexp; deploying it changes only which posts the
next `cmd/tg-ingest` run enqueues. Rollback is reverting the commit — already-extracted jobs
stay in the catalogue and are unaffected.

## Open Questions

None. Channel liveness, preview availability, marker scoring, and the geography gap were all
measured before this document was written.
