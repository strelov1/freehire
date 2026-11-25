## Why

CV seed still loses portfolio project identity and skips certifications, and can seed an empty work history when the experience bank has not imported yet. After fixing contact blanking, candidates still open tailored CVs with projects reduced to nameless job rows (no link) and certs missing — the same class of "the résumé had it, the CV does not" failure.

## What Changes

- Preserve portfolio project **link** (and kind) when importing into the experience bank, and when projecting banked projects back into a CV seed as `Document.Projects` — not as stripped `Experience` rows.
- Seed **certifications** from the structured résumé into `Document.Certifications` the same way skills are mapped.
- When composing a seed, if the bank has no employments yet but the current structured résumé still carries experience, **fall back** to the structure's experience for that seed only (bank remains source of truth once populated).
- Keep publishable-provenance rules: `agent_inferred` still never reaches a seeded CV.

## Capabilities

### New Capabilities

- (none)

### Modified Capabilities

- `experience-bank`: portfolio import/projection must retain project link and surface projects as projects (not jobs) for CV seed / WorkHistory consumers that need the project shape.
- `cv-builder`: deterministic `Seed` mapping includes certifications; work history prefers the bank and falls back to the structured résumé when the bank is empty; projects land in the projects section with name, link, and bullets.
- `cv-tailoring`: bootstrap / reset-from-résumé seed composition follows the same bank-first / structure-fallback and project/cert mapping (shared seeder path).

## Impact

- `internal/experience` — Employment (or sibling field) for project URL; `EntriesFromResume`; `WorkHistory` / a projects projection used by seed.
- `internal/cv/seed.go` — map certifications; accept bank-split jobs vs projects.
- `internal/handler/cv_seed.go` — empty-bank fallback to structure experience; compose projects from bank + structure without duplicating.
- Tests across experience import, Seed, bankedSeeder, and tailor/reset seed paths.
- No migration required if the link column is nullable on existing employments (backfill best-effort from structure on next import/FillBlanks); otherwise a small migration for `experience_employments.link` (or equivalent).
