## Context

All three Typst templates and the HTML preview render a Projects section heading when `data.projects` is non-empty (`sectionLabel("Projects")` / `section("Projects")` / `sectionHeading('Projects')`). There is no document field for that heading. The tailor agent nonetheless debated inventing `"section": "PROJECTS"` and whether to put Sandrock under experience.

Meanwhile `cv-seed-projects-experience` already maps bank `kind=project` (and structure projects) into `projects[]`. Base CV for the local user still shows `projects: []` while tailored copies retain Sandrock — so either seed never saw the project (not in bank / stale structure) or an agent only patched the tailored copy.

## Goals / Non-Goals

**Goals:**

- Agent instructions make heading ownership and `projects` placement unambiguous.
- Base (and reset) documents include portfolio projects when source data has them.
- Users stop getting wrong agent advice about hand-adding PROJECTS titles.

**Non-Goals:**

- Redesigning classic-ats typography for PROJECTS.
- Renaming DB columns or wire for experience bank (already done in `experience-project-name-wire`).
- Auto-migrating every historical tailored CV that misfiled projects under experience (one-off ops if needed).

## Decisions

1. **Prompt + tool description only for agent behavior** — hard enforcement stays path policy (`projects[i].…` vs `experience[i].…`); we do not add a server-side "reject heading fields" unless inventing such paths becomes common.
2. **Seed remains source of truth for base projects** — if structure or bank has projects and base `projects` is empty after seed/heal, that is a seed bug to fix in this change; agent edits remain the path for job-specific copies.
3. **No template change** unless product later asks for stronger visual weight on classic-ats.

## Risks / Trade-offs

- Stronger prompt text can crowd the tailor system prompt; keep it to a short bullet cluster, not a lecture.
- Fixing base seed does not rewrite existing tailored documents that already have correct `projects[]`.

## Migration Plan

Ship prompt/tool text with the next deploy. Re-seed or heal empty base `projects` when bank/structure provides them (same heal/seed paths as header work where appropriate). No DB migration.

## Open Questions

- None blocking: optional classic-ats visual tweak deferred.

## Local data note (ops, Aug 2026)

User 1 base CV had empty `projects` while tailored copies already listed Sandrock under `projects[]`. The experience bank had no Sandrock row and structure seed had nothing to place there — so seed was not wrong; the entry only exists on those tailored documents. Do **not** invent it onto the base: add via bank (`kind=project`) then reseed/reset, or `cv_edit` on the base / ask the tailor on a copy.
