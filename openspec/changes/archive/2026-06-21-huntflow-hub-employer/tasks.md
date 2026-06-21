## 1. Division parser (pure function, TDD)

- [x] 1.1 RED: add `huntflow_test.go` cases for `companyFromDivision(division, fallback)` —
  `Mirai · Partners · Vacancies`→`Mirai`; `Remote · Sparkland · Partners · Vacancies`→`Sparkland`;
  `Fluently • Partners • Vacancies`→`Fluently`; `NDA (Hedge Fund) · Partners · Vacancies`→`NDA (Hedge Fund)`;
  empty / no-`Partners` / `Partners`-first → `fallback`.
- [x] 1.2 GREEN: implement `companyFromDivision` in `huntflow.go` (normalize `•`→`·`, split, trim,
  take segment before `Partners`, else fallback).

## 2. Hub flag on the config entry

- [x] 2.1 Add optional `Hub bool` field (`yaml:"hub"`) to `CompanyEntry` in `source.go`, documented
  like `Region` (optional, provider-specific). Confirm `Config.Validate` is unaffected.

## 3. Wire the parser into the adapter (TDD)

- [x] 3.1 RED: add a `detail()`/`Fetch` test proving a hub entry (`Hub: true`) sets `Company` from the
  vacancy `division`, and a non-hub entry keeps `Company = e.Company`, using a devalue fixture.
- [x] 3.2 GREEN: read `division` in `detail()`; when `e.Hub`, set `Company = companyFromDivision(div, e.Company)`,
  else `Company = e.Company`.

## 4. Enable on AlumniHub

- [x] 4.1 Set `hub: true` on the `alumnihub-career` entry in `sources/huntflow.yml`.

## 5. Verify

- [x] 5.1 `go build ./... && go vet ./... && go test ./internal/sources/` green.
- [x] 5.2 Live smoke: run the huntflow adapter against `alumnihub-career` and confirm real
  employers (Fluently/Mirai/Sparkland/…) instead of `AlumniHub`.
