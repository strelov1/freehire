## 1. Normalized-key match

- [ ] 1.1 Extend `SuppressAggregatorDuplicatesForCompany` in `internal/db/queries/jobs.sql`:
  compute a second key `ntitle2` on both the `ats` and `agg` CTEs — decode the common HTML
  entities, strip one trailing ` - `/` | `/` — ` segment when a non-empty base remains, then
  the existing lowercase-and-collapse — and add it as an OR match path in the `target` join
  (`ntitle = ntitle OR ntitle2 = ntitle2`), keeping the country gate and the LEFT JOIN + MIN
  shape. Regenerate sqlc (`make sqlc`).

## 2. Tests

- [ ] 2.1 Extend the integration suite (`internal/db/aggregator_dedup_integration_test.go`):
  ATS `... - Leisure` suffix matches the bare aggregator title; `F&amp;B` vs `F&B` matches;
  exact-key cases still pass (regression); a hyphen-without-spaces title is not shredded;
  ATS stays canonical and failover-on-close still holds under a normalized-key match.

## 3. Verification

- [ ] 3.1 On the integration harness (or a scratch DB seeded from a gulftalent/Oracle sample
  like Marriott), confirm the normalized key suppresses the suffix/entity-mangled aggregator
  copies that the exact key missed, and that the exact-key count is unchanged.
