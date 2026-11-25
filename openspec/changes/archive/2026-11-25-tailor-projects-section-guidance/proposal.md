## Why

Users (and the tailor agent) keep treating the rendered **PROJECTS** heading as something that must be injected into CV JSON, and sometimes park portfolio work under `experience` instead of `projects`. Templates already emit the section title whenever `data.projects` is non-empty; inventing heading fields or misfiling entries does nothing useful and burns chat rounds. Local tailored CVs already carry Sandrock under `projects[]`, but the base CV often has an empty `projects` array, so opening the base or resetting from bank can drop portfolio entries unless seed/agent put them in the right place.

## What Changes

- Document and teach (system prompt + `cv_edit` tool description) that **section headings are template-owned**; agents MUST NOT invent heading/title fields for Projects / Experience / Education.
- Document that **portfolio / personal / side projects** belong in `projects[]` (with `name`, optional `link`, `bullets`), not under `experience[]`.
- Ensure the **base CV** receives portfolio entries in `projects[]` when they exist on the structured résumé or bank (`kind=project`), matching the seed rules already introduced for banked projects — fix any gap that leaves base `projects` empty while tailored copies still hold them.
- No Typst/HTML template change required for the heading itself (already correct); optional follow-up only if classic-ats contrast proves too weak in product feedback.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `assistant-agent-runtime`: Tailor-session prompt and `cv_edit` guidance MUST state template-owned headings and the `projects` vs `experience` placement rule.
- `cv-tailoring`: Seeded/base documents MUST keep portfolio work in `projects[]`; agent-facing copy MUST not instruct inventing section titles.

## Impact

- `internal/assistant/prompt.go` (`tailorPrompt` / related)
- `internal/handler/assistant_cv_tools.go` (`cv_edit` Description)
- Possibly `internal/handler/cv_seed.go` / bank seeder if base still omits structure/bank projects
- Prompt / seed unit tests; no API wire shape change
