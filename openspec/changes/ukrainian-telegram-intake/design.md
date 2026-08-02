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
- No schema change and no new dependency. The rollout is not a no-op — see Migration Plan.

## Decisions

### Extend the existing regexp rather than restructure it

Ukrainian becomes a fourth commented block alongside RU and EN.

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

Dropped on the numbers above: `наймаємо` and `зарплатн` (never fire — `зарплатн` is already
covered by the RU prefix `зарплат`), `потрібен` and `відгук` (1:1 signal to noise).

**`грн`/`₴` was dropped after code review, and the scoring above is why it nearly shipped.**
The table counts *marker matches inside job channels*, which is a proxy for "rescued a
vacancy" — and the proxy breaks on this cohort, because five of the seven channels are
editorial. Reading the posts the alternative admitted on its own: seven of seven are DOU Day
Picnic ticket prices, an army fundraiser, and a vinyl raffle. Zero vacancies. The hryvnia is
low-denomination, so the amount pattern's three-digit floor cannot separate «500 грн» for a
ticket from a salary, where the same floor works for «250 000 руб» and «$120k». Its measured
value on the shipped cohort is negative, so the currency alternation is left untouched.

Adopted, therefore: `вакансі`, `шукає`, `запрошуємо`, `стажуванн`, `досвід роботи`. The
regexp writes `шукає` rather than `шукаємо|шукає` — the shorter stem is a prefix of the
longer, so it matches everything the pair did, while still excluding `шукаю`, the
job-seeker form.

The asymmetry settling the borderline cases: a false positive costs one LLM call returning
`{"jobs": []}`; a false negative loses a vacancy permanently and silently.

### Ukrainian geography goes into the curated map, not the generated one

Two additions to `nameToCountry` in `internal/location/dictionaries.go`, in the blocks that
already hold `kyiv` and `київ`: the oblast centres in Latin (Ukrainian and Russian
transliteration), and the same in Cyrillic plus `україна`/`украина`.

A name is omitted where GeoNames places the alias in more than one country, the precedent the
file already sets for `georgia` (the US state). The two blocks apply that test at different
strengths, because a Latin location field can come from anywhere while a Cyrillic one comes
from a Cyrillic-writing source: `odesa`/`odessa`, `lutsk`, `cherkasy`, `donetsk`, `nikolaev`
are absent from the Latin block, and only the ones colliding with Belarus, Bulgaria, or
Russia (`луцьк`, `черкассы`, `николаев`) are absent from the Cyrillic one. `donetsk` is
ambiguous in both scripts and appears in neither. Those cities keep resolving to a bare city
name — under-resolving is what the never-guess contract asks for.

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

- **More posts reach the metered LLM stage.** → Bounded by seven channels: 16 posts to 39
  across their latest 136. The five DOU verticals are editorial, so some of the additions are
  course digests and event invitations; each costs one call returning `{"jobs": []}`, and the
  extraction prompt already lists "course ad" as a negative.
- **A broader alternation could admit noise in channels that already work.** → Extending an
  alternation is monotonic: it can only add matches, so this is the only way the change can do
  harm. The before/after measurement on the eight production RU channels must show no change.
- **`досвід роботи` and `стажуванн` can appear in course advertising.** → Accepted given the
  recall asymmetry; each is one alternative to remove if it degrades. Every adopted
  alternative has a test that fails when it alone is deleted, so removing one is a
  one-line change with immediate feedback.
- **The docs mirror can drift from the YAML.** → Both edited in the same commit; the counts are
  in sync today (88 rows against 88 entries) and verified after the change (95).
- **Ukrainian city names are added by hand.** → Confined to oblast centres; long-tail towns
  keep resolving to a bare city name, the same as any other country's long tail.
- **A curated name could collide with a place in another country.** → Every added key was
  checked against `cities15000.tsv`; the ambiguous ones are omitted and listed in the code
  comment. Verified after the fact: `Parse("Odesa, TX")` still resolves to `us` alone.

## Migration Plan

No schema migration. The rollout is not a no-op, though: `internal/location/AGENTS.md` states
that a dictionary change needs a re-derive to reach existing rows, because geography lives as
Meilisearch facets rather than SQL columns. Without it the prefilter half takes effect on the
next `cmd/tg-ingest` run while every job already in the catalogue with a Ukrainian location
keeps its empty `countries`/`regions` — the precise benefit this change promises.

Order:

1. Deploy.
2. `cmd/backfill-derive` — re-derives the facet columns from the new dictionary.
3. `make reindex` — rebuilds the search index so the facets are served.

Never stack step 3 with `reindex-companies`, and stop `freehire-reindexw.timer` before a
manual reindex.

Rollback is reverting the commit; the same two steps then restore the previous facets.
Already-extracted jobs stay in the catalogue either way.

## Open Questions

None. Channel liveness, preview availability, marker scoring, and the geography gap were all
measured before this document was written.
