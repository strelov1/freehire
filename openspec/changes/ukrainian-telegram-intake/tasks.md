## 1. Prefilter recognises Ukrainian

- [x] 1.1 Add failing Ukrainian cases to `internal/telegram/prefilter_test.go`, in the two
  tables that already exist. Pass: «Вакансія: Golang розробник», «Шукаємо QA у продуктову
  команду», «Досвід роботи від 2 років, зарплата 60 000 грн», «Запрошуємо на стажування».
  Reject: «Дайджест новин тижня», «Знижка 50% на курс — встигни записатись». Confirm the pass
  cases fail today and the reject cases already hold.
- [x] 1.2 Add the Ukrainian marker block to the regexp in `internal/telegram/prefilter.go`
  (`вакансі|шукає|запрошуємо|стажуванн|досвід роботи`) with a comment recording why `ваканси`
  does not reach it. Tests green.

  `грн|₴` was scored as a candidate and **not** shipped: reading the posts it admitted on its
  own showed seven event tickets, fundraisers and a raffle, and zero vacancies. A reject test
  pins that decision.

## 2. Ukrainian geography resolves to a country

- [x] 2.1 Add failing cases to `internal/location/location_test.go`, mirroring the existing
  `Київ` case: `Львів`, `Харків`, `Lviv, Ukraine`, and `Україна` → the expected `Geo`
  (countries `["ua"]`, regions `["eu"]`, city where a city is named).
- [x] 2.2 Add the Latin entries (oblast centres, Ukrainian and Russian transliteration) to
  `nameToCountry` in `internal/location/dictionaries.go`, in the CIS block beside `kyiv`.
  Check every key against `cities15000.tsv` and omit the ones GeoNames places in more than
  one country. Tests for the Latin cases green.
- [x] 2.3 Add the Cyrillic entries — the same cities in Ukrainian and Russian spelling, plus
  `україна`/`украина` — in the Cyrillic block beside `київ`. All of 2.1 green.
- [x] 2.4 Teach `cityMarkerPrefixes` the Ukrainian city marker (`м.`, `місто `) alongside the
  Russian `г.`/`город `, with a regression case proving a city merely starting with `м`
  (`Мурманск`) is untouched. Found in review: `м. Львів` resolved to nothing at all.

## 3. Channels and their mirror

- [x] 3.1 Add the seven channels to `sources/telegram.yml` under two commented headings:
  `naymarnya` and `halepnyirecruiting` as `board`, and `devops_dou`, `dou_qa`, `frontend_dou`,
  `gamedev_dou`, `junior_dou_ua` as `authored`. Record in the comment that the cohort was
  verified 2026-08-01 and that `wwjobs` was held back as stale.
- [x] 3.2 Add the dated section to `docs/telegram-channels.md` and update the header count from
  `88 channels (12 authored, 76 board)` to `95 channels (17 authored, 78 board)`. Note there
  why the source list's 97 group chats are unusable. Verify the table row count matches the
  YAML entry count.

## 4. Rollout

- [ ] 4.0 After deploy, run `cmd/backfill-derive` and then `make reindex`. Geography lives as
  Meilisearch facets, so a dictionary change reaches existing rows only through a re-derive
  (`internal/location/AGENTS.md`); without it only newly-ingested jobs get `ua`/`eu`. Stop
  `freehire-reindexw.timer` first and never stack the reindex with `reindex-companies`.

## 4. Verification

- [x] 4.1 Run `go test ./...` and `go test -tags=integration ./internal/db/`. Both green.
- [x] 4.2 Re-run the prefilter measurement against live `t.me/s` previews: `naymarnya` and
  `halepnyirecruiting` pass materially more posts than before, and the eight production RU
  channels (`it_vakansii_jobs`, `vakansii_it`, `jobforjunior`, `huntmejob`, `normrabota`,
  `zrabota`, `budujobs`, `product_jobs`) show no change. Record the numbers.

  Measured 2026-08-01 with the shipped `LooksLikeVacancy` against the pre-change regexp,
  over live `t.me/s` previews:

  | cohort | posts | before | after | delta |
  |---|---|---|---|---|
  | Ukrainian (7 channels) | 136 | 16 | 39 | **+23** |
  | production RU (8 channels) | 159 | 120 | 120 | **+0** |

  Per channel, the largest gains are `junior_dou_ua` +7, `naymarnya` +6, and
  `halepnyirecruiting` +3 (from zero). `gamedev_dou` gained nothing — its latest 20 posts
  carry no vacancy at all, which is a property of the window, not of the filter. Every
  production channel moved by exactly zero, which is the safety property this task exists
  to check.
