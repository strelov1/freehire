## Why

The Telegram crawl covers 88 channels, all Russian- or English-language, and the prefilter
that guards the extraction queue carries hiring markers in those two languages only. A
Ukrainian post is silently discarded: `вакансія` does not match the Russian alternative
`ваканси`, because Cyrillic `і` (U+0456) and `и` (U+0438) are different runes. Measured on
live channel previews, the current prefilter passes 3 of 18 posts on `naymarnya` and 0 of 18
on `halepnyirecruiting` — so onboarding Ukrainian channels without fixing the filter would
look like the channels are weak rather than the filter blind.

## What Changes

- Ukrainian hiring markers join the prefilter regexp as a fourth language block: `вакансі`,
  `шукає`, `запрошуємо`, `стажуванн`, `досвід роботи`. Candidates that never fired or that
  scored 1:1 signal-to-noise over 306 live posts are excluded, as is `грн`/`₴` — reading the
  posts it admitted showed event tickets and fundraisers, not salaries.
- Ukraine's oblast centres and the native country spelling enter the curated
  `nameToCountry` map, in Latin and in both Cyrillic spellings, minus the names GeoNames
  places in more than one country. Today only `kyiv`/`киев`/`київ`
  are present, so a Lviv or Kharkiv vacancy lands with a city name but no country and no
  region — invisible to `country=ua` and to the `eu` region filter.
- Seven vetted Ukrainian channels join `sources/telegram.yml`: two job boards
  (`naymarnya`, `halepnyirecruiting`) and five DOU editorial verticals (`devops_dou`,
  `dou_qa`, `frontend_dou`, `gamedev_dou`, `junior_dou_ua`).
- `docs/telegram-channels.md`, which declares itself a mirror of the YAML, gains a dated
  section and an updated header count.

No breaking changes. No schema change, no migration.

## Capabilities

### New Capabilities

None. Both affected areas already have specs.

### Modified Capabilities

- `telegram-ingest`: the prefilter requirement gains an explicit obligation that marker
  coverage span the languages the configured channels publish in. The gap this change fixes
  was not a coding slip — it was an unstated invariant, so it belongs in the spec rather than
  in a comment.

`job-geography` is deliberately **not** modified. Its requirement already says the parser
resolves tokens against curated dictionaries and emits only what those dictionaries contain,
never guessing. Adding Ukrainian entries is data under an unchanged rule; changing the spec
would imply the rule changed.

## Impact

- `internal/telegram/prefilter.go`, `internal/telegram/prefilter_test.go`
- `internal/location/dictionaries.go`, `internal/location/location_test.go`
- `sources/telegram.yml` — 88 channels → 95
- `docs/telegram-channels.md` — mirror and header count
- Cost: the prefilter admits more posts to the metered LLM extraction stage, bounded by the
  seven new channels — 16 posts to 39 across their latest 136. Not all of the additions are
  vacancies: the five DOU verticals are editorial, so course digests and event invitations
  reach the LLM and come back as `{"jobs": []}`. That is one call each, and the extraction
  prompt already lists "course ad" as a negative.
- No effect on the 88 channels already crawled: the same measurement over eight of them
  showed zero posts rescued by Ukrainian markers, because they publish in Russian.
