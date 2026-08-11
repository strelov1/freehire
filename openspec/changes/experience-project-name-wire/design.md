## Context

See proposal.md. Storage already parks the project title in `experience_employments.company`; CV seed already remaps `e.Company` → `Project.Name`. The lie is only on the employment JSON (`json:"company"`) and UI that prints `employment.company` for every kind.

## Goals / Non-Goals

**Goals:**
- Kind-aware wire: project → `name`, job → `company`.
- Write accept `name` or legacy `company` for projects.
- TS + Experience UI aligned.

**Non-Goals:**
- Renaming the DB column or Go field `Company`.
- Splitting projects into a separate table.
- Changing CV `Document.Projects` (already name-shaped).

## Decisions

### 1. Custom JSON on `experience.Employment` (not a parallel DTO)

**Choice:** Implement `MarshalJSON` / `UnmarshalJSON` on `Employment` so every HTTP and tool path that embeds the type gets the kind-aware shape without forking handlers.

**Why:** List/create/update all nest `experience.Employment`; a one-off handler DTO would drift. Storage and domain methods keep using `.Company`.

**Alternatives:** Handler-only DTO (more duplication). Rename Go field to `Place` (wider churn than option A asked for).

### 2. Response omits `company` for projects when the label is present as `name`

**Choice:** For `kind=project`, emit `name` from the stored company value; leave `company` absent (omitempty / not set). For jobs, emit `company` only.

**Why:** Spec forbids presenting the label as `company` for projects. Dual fields would keep the lie alive.

### 3. Unmarshal: `name` wins over `company` for projects; either fills `.Company`

**Choice:** If `kind` is project (or inferred), prefer `name`, else fall back to `company`. Jobs ignore `name` or map `name` only if we want forgiveness — prefer ignore `name` on jobs to avoid accidental mis-posts.

**Why:** One rollout window for clients still POSTing `company`.

### 4. Contracts / UI

**Choice:** Regenerate or hand-update `ExperienceEmployment` so projects document `name?` and jobs `company?`; `ExperienceBankView` uses `name` when `kind==='project'`.

**Why:** gen-contracts may not understand custom marshal — verify; if it still emits only `company`, adjust the generator hint or the TS type by hand with a comment pointing at the marshaler.

## Risks / Trade-offs

- **[Risk] gen-contracts still documents `company` only** → Mitigation: check generator; patch TS union or documented fields if needed.
- **[Risk] External API clients reading `company` on projects** → Mitigation: note mild break in proposal; write alias keeps POST working.
- **[Risk] Match key still `(company, role)` in DB** → Acceptable; identity unchanged.

## Migration Plan

Deploy API + web together. No DB migration. Optional: leave write alias indefinitely (cheap).

## Open Questions

None — option A chosen by product.
