## Why

CV appearance (template, font, font size, line height, margins) is only editable per-CV inside
the tailoring workspace, and every new CV starts from the same hardcoded values
(`classic-ats`, 0.5in margins, template-default typography). A candidate who has already tuned
their appearance once has to redo it by hand on every new base CV or reset. There is no place to
say "this is how I always want my CV to look."

## What Changes

- Add a new per-user "CV appearance defaults" record (template + typography + margins), editable
  from a new settings screen reachable from the CV list (`/my/cvs`).
- When a base CV is created with no explicit appearance chosen for it, seed it from the user's
  saved defaults instead of the hardcoded system defaults, falling back to the system defaults
  when the user has none saved. An explicit `template_id` on the create request still wins over
  the saved default.
- Saving new defaults never touches any already-created CV — a CV's appearance is independent of
  the defaults from the moment it is created, exactly as it is independent of every other CV
  today.
- Generalize the existing template gallery component to a controlled (non-persisting) mode so it
  can be reused on the new settings screen without touching its existing self-persisting
  behaviour in the tailoring workspace.

Explicitly out of scope: re-applying saved defaults to an already-open/existing CV (no
"reset to my defaults" action in the tailoring workspace), and a new top-level navigation entry
(the settings screen is reachable only via a button on the CV list page).

## Capabilities

### New Capabilities
- `cv-appearance-defaults`: per-user default template/typography/margins — storage, API, and the
  settings screen that edits them.

### Modified Capabilities
- `cv-builder`: base-CV creation (seeding from résumé or empty) and the "default template" /
  "unset margins default" behavior now consult the user's saved appearance defaults before
  falling back to the system hardcoded defaults.

## Impact

- New DB table `cv_appearance_defaults` (migration + sqlc queries).
- `internal/candidate/cv`: new file for the defaults type/store methods, reusing existing
  `Style`/`Margins`/template validation; the three existing base-CV creation call sites
  (`Store.Tailor`, `cv_reset.go`, `CreateCV` handler) read the saved defaults.
- `internal/api/handler`: new `GET`/`PUT /api/v1/me/cv-appearance-defaults` routes.
- `web/src/lib/tailor/TemplateGallery.svelte`: gains a controlled mode alongside its existing
  `cvId` self-persisting mode.
- New route `web/src/routes/my/cvs/appearance/+page.svelte` and an entry-point button on
  `web/src/lib/components/cv/CvList.svelte`.
- No changes to `accountNav.ts` / `accountNavIcons.ts`, no changes to tailored (job-bound) CV
  creation (it already inherits appearance from the base CV).
