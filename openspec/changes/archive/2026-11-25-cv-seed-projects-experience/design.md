## Context

See proposal.md for motivation. After contact blanking, seed composition still:

1. Imports portfolio projects into the bank as `KindProject` with `Company = name` and **drops `Link`**.
2. Flattens every employment through `WorkHistory` into `[]resumeextract.Experience`, so `bankedSeeder` overwrites `st.Experience` with jobs **and** projects-as-jobs while leaving `st.Projects` from the structure — duplicates or stripped rows.
3. Never maps `Structured.Certifications` in `cv.Seed`.
4. Clears structure experience even when the bank is empty (import pending or failed), so a seed can ship with zero roles despite a current extract.

The bank already distinguishes `job` vs `project` kinds; the gap is persistence of URL and the seed composition path.

## Goals / Non-Goals

**Goals:**

- Persist and round-trip project URLs on banked project employments.
- Compose CV seeds with jobs in Experience, projects in Projects (with links), certs from structure.
- Bank-first experience with structure fallback only when the bank has zero employments.
- Keep publishable-atom gating unchanged.

**Non-Goals:**

- Changing fit-analysis `Professional` scoring semantics beyond what falls out of keeping a flat work-history adapter for that path.
- Rebuilding the experience UI or import matching rules (company/role FillBlanks stays).
- Backfilling links for historical bank rows beyond best-effort FillBlanks on the next import.
- Surfacing certifications in the experience bank (they stay structure-owned).

## Decisions

### 1. Add `link` on `experience_employments`

**Choice:** Nullable/empty `text` column `link` on `experience_employments`, exposed on `Employment`, sanitized like other short strings, included in create/update/FillBlanks.

**Why:** The bank already owns project identity; a sidecar table or stuffing the URL into `summary` would be worse. Specs require FillBlanks (empty → fill, non-empty → keep).

**Alternatives:** Store link only on atoms — rejected (link is employment identity, not a claim). Encode in `company` — rejected (breaks display and matching).

### 2. Split seed projection; leave Professional flat

**Choice:** Add a bank projection for CV seed (or extend the reader the seeder uses) that returns job-shaped history and project-shaped rows separately. `WorkHistory` / `Professional` MAY keep flattening places into `[]Experience` so fit analysis still sees project evidence without a second scoring shape.

**Why:** Seed is the broken consumer; fit already treats project places as career history. Forcing Professional onto a projects array would expand this change into matchanalysis without fixing the user-visible CV bug.

**Alternatives:** Make `WorkHistory` job-only and teach Professional to merge structure projects — rejected (projects then disappear from fit when structure is stale). Dual-write into both `st.Experience` and `st.Projects` from one flat list — rejected (loses kind).

### 3. Empty-bank fallback is employment-count, not “jobs empty”

**Choice:** If `ListEmployments` (or equivalent) returns zero rows, restore `st.Experience` from the structure for seed. If any employment exists (job or project), clear structure experience and use bank jobs only for Experience; projects from bank projects or structure fallback independently when bank has no project-kind rows.

**Why:** Matches “bank is source of truth once populated” without resurrecting deleted jobs when the user still has projects only.

**Alternatives:** Fall back only when WorkHistory is empty after filtering jobs — weaker (project-only bank would still wipe structure jobs incorrectly if we cleared experience always). Merge bank ∪ structure — rejected (resurrects deletions).

### 4. Certifications only in `cv.Seed`

**Choice:** Map `s.Certifications` → `Document.Certifications` in `Seed`; no bank involvement.

**Why:** Already extracted and present on `Structured`; seed simply never copied them.

### 5. Seeder interface growth

**Choice:** Prefer extending `workHistoryReader` (or replacing it with a small seed-bank reader) so `bankedSeeder` can set `Experience` and `Projects` without importing sqlc into `cv`. `cv.Seed` stays a pure Structured → Document map.

**Why:** Keeps the existing seam: `internal/cv` does not know the bank.

## Risks / Trade-offs

- **[Risk] Existing project rows have empty links** → Mitigation: next import FillBlanks; no mandatory backfill worker.
- **[Risk] Structure projects + bank projects double-count if both paths fill** → Mitigation: when any project-kind employment exists, use bank projects only; otherwise structure.
- **[Risk] Professional still shows projects as job-shaped rows** → Accept for this change; document as non-goal. Fit continues to see the evidence.
- **[Risk] Migration on prod volumes** → Same pattern as other ALTER TABLE migrations: new file under `migrations/`, apply before deploy.

## Migration Plan

1. Add migration for `experience_employments.link` (default `''`).
2. Deploy code that reads/writes `link`, splits seed projection, maps certs, applies empty-bank fallback.
3. Rollback: column can remain unused; older code ignoring `link` is safe. Reverting seed logic alone reintroduces the bug but does not corrupt data.

## Open Questions

None — FillBlanks for link and independent project vs job fallbacks are fixed above.
