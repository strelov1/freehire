## Context

See proposal.md - Why. Relevant existing state:

- Appearance (`template_id`, `Style{font_family, font_size, line_height}`, `Margins{top, right,
  bottom, left}`) lives on every `cvs` row and is validated/clamped by `internal/candidate/cv`
  (`Style`/`Margins.sanitized()`, `template.go`'s registry). `internal/candidate/cvedit` is the
  sole writer for an existing CV's document.
- Exactly three call sites seed a fresh **base** CV with the hardcoded system defaults today:
  `Store.Tailor` (`internal/candidate/cv/store.go:314`, résumé-seed branch), `cv_reset.go:132`,
  and the `CreateCV` handler (`internal/api/handler/cv.go`, ~356-390). Tailored (job-bound) copies
  always inherit `template_id`/`Style`/`Margins` from the base CV (`CreateTailored`/`Align`), so
  they need no change.
- The repo's block-layering rule (`internal/platform/arch/layering`) forbids `identity` (layer 3)
  from importing `candidate` (layer 4) types. `Style`, `Margins`, and the template/font registries
  already live in `internal/candidate/cv`.
- `TemplateGallery.svelte` is bound to a `cvId` and self-persists via `api.setCvTemplate` on
  click; `StyleSettings.svelte`/`MarginSettings.svelte` are already `$bindable`, parent-persisted
  components.
- `/my/cvs` (`CvList.svelte`) is the CV list page; there is no unified top-level `/my/*` settings
  section — each area is its own nav entry (`accountNav.ts`), and the user asked for this feature
  to live inside the CV list area instead of adding one.

## Goals / Non-Goals

**Goals:**
- One durable per-user record holding appearance defaults, validated with the exact rules a CV
  document already uses.
- Apply it at the three existing base-CV creation call sites, non-invasively.
- Reuse `StyleSettings`/`MarginSettings` unchanged; generalize `TemplateGallery` rather than
  forking it.

**Non-Goals:**
- Re-applying defaults to an existing/open CV ("reset to defaults" in the tailoring workspace).
- A new top-level `/my/*` navigation entry.
- Any change to tailored (job-bound) CV creation — it already inherits from the base CV.

## Decisions

**Where the data lives: `internal/candidate/cv`, not `internal/identity/userprofile`.**
`userprofile` already holds an analogous per-user preference (`LocationPreferences`), which is
the closest existing pattern for "a user-level default." But `identity` sits below `candidate` in
the layering table, so it cannot import `Style`/`Margins`/the template registry — and duplicating
that validation/registry logic in `identity` would drift from the copy in `cv.go`/`template.go`
the way the repo's own `normalize` package warns against for company legal-form vocabularies.
Keeping the defaults in `internal/candidate/cv` (new file, same package) means
`SetAppearanceDefaults` calls the exact same `Style`/`Margins.sanitized()` and
`ResolveTemplate`/`TemplateIDs()` the CV document path already uses — no second copy to drift.

**Storage shape: one new table, not a column on `users`.** `cv_appearance_defaults` mirrors the
existing `cvs` table's `template_id`/`style jsonb`/`margins jsonb` shape (PK `user_id`, no
`title`/`job_id`). A dedicated table keeps this optional, deletable, and separate from the
`users` row's own migration history — and matches how `cvs` itself is already structured, so the
same jsonb marshal/sanitize helpers apply directly.

**GET always returns a concrete shape.** Absent saved defaults, `GetAppearanceDefaults` returns
`(system defaults, false, nil)` rather than a zero/absent value — the settings screen always has
something to show and to diff "unsaved changes" against, with no special-cased empty state.

**Application point: seed values passed into `Store.Create`, not a new hook in `Sanitize`.**
The three call sites already assemble a `Document` (plus a `templateID` string) before calling
`Store.Create`/`CreateTailored`. Each is changed to look up the caller's saved defaults first and
substitute them for today's hardcoded `DefaultTemplateID`/`DefaultMargins()`/zero-value `Style`
before that existing call — `Sanitize` still runs exactly as before, on whatever values arrive.
This is a smaller, more local change than teaching the sanitizer itself about per-user state, and
keeps `Sanitize`'s meaning ("fill an actually-unset value with the system default") intact for
every other code path that calls it (edits on an existing CV keep meaning "unset" as 0.5in/template
default, never as "this user's saved default" — only the initial seed at creation reads the saved
defaults).

**`CreateCV` handler's explicit `template_id` still wins.** Its request already accepts a caller
`template_id` (e.g. a "start blank, pick a template first" flow in the client). Saved defaults
apply only when the request didn't specify one; typography/margins have no per-request override
today, so saved defaults (or system defaults) always apply to them at creation.

**`TemplateGallery.svelte` gains a controlled mode instead of forking.** New optional props
`value`/`onChange` sit alongside the existing `cvId` prop: when `cvId` is given, behavior is
byte-for-byte what it is today (fetch + self-persist via `api.setCvTemplate`); when `value` is
given instead, the component is purely presentational — it renders the same grid/thumbnails and
calls `onChange(id)` without any API call. One component keeps owning "what templates exist and
what their thumbnails look like."

**Settings screen uses explicit save, not the tailoring workspace's autosave.** Every other
`/my/*` settings surface in this app (e.g. profile forms) saves on an explicit action, not on
every keystroke; the tailoring workspace's 800ms-debounce autosave is particular to a live
document a user is actively drafting. A defaults screen with no live preview to justify
autosave's responsiveness follows the more common `/my/*` pattern instead.

## Risks / Trade-offs

- **A user with saved defaults who later changes them expects nothing to move retroactively** —
  this is deliberate (see proposal's excluded scope) but worth stating plainly in the settings
  screen's copy so it isn't read as a bug.
- **Three separate call sites must be kept in sync** if a fourth base-CV-creation path is ever
  added — mitigated by putting the "resolve effective appearance for a new CV" logic in one
  helper in `internal/candidate/cv` that all three call, rather than duplicating the lookup.
- **`GET` returning system defaults when nothing is saved** means a client cannot distinguish "no
  defaults saved" from "saved defaults that happen to equal the system defaults" — acceptable,
  since nothing in this feature needs that distinction (there is no "reset to system default"
  affordance to gate on it).

## Migration Plan

Additive: new table, new endpoints, new route, new optional component props. No existing data
changes, no existing endpoint's response shape changes. Ship in one deploy; no rollback beyond
the standard revert (the new table stays empty and unread if the code is rolled back first).
