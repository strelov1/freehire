## Why

After a résumé-vs-vacancy fit analysis, the user has a verdict, gaps, and a per-requirement
coverage table — but no way to act on it. They should be able to tailor their CV to that
specific vacancy in a live agent session that reframes existing experience and, for genuine
gaps, asks the candidate before adding anything. This change delivers the freehire-side
contract (data model + API + typed wire shapes + entry point) that the roy/freehire-agent
session drives; it wires up the `cvs.job_id` seam the schema already anticipated.

## What Changes

- Introduce a **two-tier CV model** over the existing `cvs` table: a **base CV** (`job_id = NULL`,
  the editable canonical résumé, seeded from `structured_resume` via existing `cv.Seed`) and a
  **tailored CV** (`job_id = <vacancy>`, a per-vacancy copy that receives all tailoring edits).
- Add a **field-level patch operation** on a CV document: a pure `cv.Apply(doc, patch)` mirroring
  `cv.Seed`, exposed via `PATCH /api/v1/me/cvs/:id`. Patches address specific fields
  (`summary`, `experience[i].bullets` incl. add/replace/remove/reorder, skill groups, header
  fields); the whole document is never re-emitted. Every patch runs through `cv.Document.Sanitize`
  (bounds + prompt-injection guard).
- Add a **tailoring bootstrap** `POST /api/v1/me/cvs/tailor` `{job_slug}` that reads the cached
  `jobfit.Analysis`, finds/seeds the base CV, creates the tailored copy, mints a short-lived
  scoped API key for the agent's CLI, and returns `{tailor_cv_id, base_cv_id, analysis, cli_token}`.
- Add a **tailoring context** read `GET /api/v1/me/cvs/:id/tailor-context` returning the verdict,
  `missing-have`/`missing-gap` requirements, recommendation, and per-dimension comments the agent
  reasons over (sourced from the cached analysis, no LLM recompute).
- Enforce the **honest wall** at the API/model level: patches carry only reframed existing content
  or candidate-confirmed facts; the API never fabricates. `missing-have` → surface existing
  evidence; `missing-gap` → only written after candidate confirmation.
- Emit the new wire types (`cv.Patch`, tailor-context) through `cmd/gen-contracts` into
  `web/src/lib/generated/contracts.ts`.
- Add the **web entry point**: a "Подогнать CV под вакансию" CTA on `/jobs/[slug]/fit`, shown only
  when a cached non-stale analysis exists, that calls the bootstrap and opens the assistant surface.
- **Beta-gated** throughout (union of the CV builder's `cvGate` and the agent's `beta_tester`).

## Capabilities

### New Capabilities
- `cv-tailoring`: the two-tier base/tailored CV model over `cvs`, the field-level patch operation
  and its endpoint, the tailoring bootstrap and context endpoints, the scoped-key minting, the
  honest-wall invariants at the model/API boundary, the typed wire contracts, and the fit-page CTA.

### Modified Capabilities
<!-- job-fit-analysis is only read (its cached output is consumed), not changed at the requirement
     level; the cvs CRUD surface has no existing capability spec. No requirement-level modifications. -->

## Impact

- **Code (this repo):** `internal/cv/` (new `cv.Patch` + `cv.Apply`, tailor bootstrap in the store
  layer), `internal/handler/cv.go` + route wiring, `internal/db/queries/cvs.sql` (create tailored
  row with `job_id`, fetch base by user), `cmd/gen-contracts`, `web/src/routes/jobs/[slug]/fit/`
  (CTA) and the assistant surface (`web/src/routes/my/assistant`, split preview). Reads cached
  `jobfit.Analysis` from `user_job_analysis`; reuses `api_keys` for scoped-key minting.
- **Cross-repo companions (NOT in this change's edit scope):** `freehire-cli` gains `freehire cv
  context|get|edit|render` subcommands that call these endpoints; `freehire-agent` gains a
  `cv-tailoring` skill that drives the dialogue. Tracked as separate changes in their repos; this
  change delivers the contract they depend on.
- **Gating:** beta only. **No breaking changes** — all additive.
- **Noted seam (out of scope):** an item-level "experience bank" as the canonical store; the base
  CV serves as the proto-bank for now.
