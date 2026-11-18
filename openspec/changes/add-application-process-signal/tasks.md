## 0. Decide whether to build (nothing below starts before this)

- [ ] 0.1 Hand-run read-only report `cmd/report-process-signal-coverage` (in the manner of
      `cmd/report-company-website-twins`; `go run`, `DATABASE_URL` only). For every open role
      cluster it counts distinct observable contributors under the rules in `design.md`, then
      prints, at assumed opt-in of 100%, 50% and 20% and floors of 5 and 10, the number of
      clusters that clear AND the share of the last 30 days' `page_uniques` on job pages that
      lands on them.
- [ ] 0.2 Run it on production and paste the table into the review. Go if at least 5% of job-page
      views clear the floor of 10 at 20% opt-in. Below that, ship only task 5 (the company figure
      on the job page) and file the rest under `.out-of-scope/` with this report as the evidence.
      The 5% is a judgement; the owner may move it.

## 1. Consent

- [ ] 1.1 Migration: `users.share_process_signal_at timestamptz` (NULL = off; the value is the
      moment of consent) and `applications.signal_hidden_at timestamptz`. Both nullable with no
      default, so neither rewrites a table. `pnpm check:sql`, `make sqlc`.
- [ ] 1.2 Failing tests, then `PUT /me/process-signal-consent` and
      `PUT /me/tracking/:slug/signal-hidden`; both idempotent; both cookie-only (a scoped API key
      must not grant consent on a person's behalf). Account deletion needs no code: the columns
      are on rows that cascade.
- [ ] 1.3 Settings switch and the tracker's per-application switch; privacy policy paragraph
      (`web/src/routes/privacy/+page.svelte`) in the same PR as the first consent write.

## 2. Aggregation (`internal/application/processsignal`)

- [ ] 2.1 Add the package to `internal/platform/arch/layering/blocks.go` and to
      `internal/application/AGENTS.md`'s package list.
- [ ] 2.2 Pure rules, tested without a pool: quantizers (band, 10% share, rounded range), the
      floors, the maturity and window arithmetic. The maturity age is read from
      `internal/job/silence`'s `applied` threshold, never restated.
- [ ] 2.3 The compute query, written as `ComputeInsightsCompanyResponse` is: on the read
      connection, joining the cluster closure through the same resolver `/copies` uses.
      Integration tests seed each exclusion: not consented, hidden application, private posting,
      `mail_external` reply, manual stage, application inside the maturity age, outside the
      window, two copies of one cluster by one member, a member who consents then revokes.
- [ ] 2.4 `insights_process_signal` table (migration) and the publish step as one more pass in
      `cmd/rollup-company`, atomic delete-and-reinsert; below-floor clusters write no row. The
      compute also reads a small moderator suppression table (migration) and logs per-cluster
      contributor counts.

## 3. Serving

- [ ] 3.1 `GET /jobs/:slug/process-signal`: resolves the slug to its ultimate owner, reads one row,
      returns `{"data": {...}}` or `{"data": null}`. Public, cacheable, never returns a stored
      raw count. Additive in `web/static/openapi.yaml`; `scripts/check-openapi-breaking.sh`.
- [ ] 3.2 Handler test that the response body contains none of the raw counts the rollup read.

## 4. Job page

- [ ] 4.1 A card on the job page, absent when the response is null. Playwright test with the
      endpoint mocked (`page.route`): present above the floor, absent below it, bands rendered as
      bands, the caveat copy shown. `pnpm exec playwright test <spec>`.

## 5. Company fallback

- [ ] 5.1 Show `companyResponseRate` on the job page when the cluster figure is absent. Depends on
      the owner's answer to open question 2 in `design.md`.

## 6. Gates

- [ ] 6.1 `gofmt -l .`, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, and the
      tagged suite for `internal/platform/db` and `internal/api/handler`.
- [ ] 6.2 `pnpm check:sql`, `make sqlc` clean diff, `pnpm check:dead`, `pnpm check:links`.
