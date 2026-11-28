## Why

Portfolio projects have a **name**, not a company. The experience bank correctly stores that label in the `company` column (one “place” table), but the HTTP/TS wire still exposes `company` for `kind=project`, so Profile and API clients call a project a company.

## What Changes

- For `kind=project` employments on the experience API (and related assistant tool projections), expose the place label as **`name`** instead of `company`.
- Keep the Postgres column and Go field `Company` as storage — no migration.
- Jobs (`kind=job`) keep `company` unchanged.
- Accept `name` on write for projects; accept legacy `company` on write as an alias into the same field so existing clients do not break mid-rollout.
- Update generated TS types and Experience UI copy so projects show/edit a name, not “Company”.

## Capabilities

### New Capabilities

<!-- none -->

### Modified Capabilities

- `experience-bank`: kind-aware wire shape for project place label (`name` vs `company`).

## Impact

- `internal/experience.Employment` JSON (custom marshal/unmarshal or a thin wire DTO) and any handler/assistant paths that emit `company` for a place.
- `cmd/gen-contracts` / `web/src/lib/types.ts` (`ExperienceEmployment`).
- `ExperienceBankView` (and any edit forms) labeling.
- **Mildly BREAKING** for consumers that read `company` on project rows; write path stays compatible via alias.
