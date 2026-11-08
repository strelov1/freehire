# Base CVs you can see, edit, duplicate and tailor from

Date: 2026-11-03
Status: approved; revised 2026-11-03 after review (owner: reuse the tailoring workspace, do not build a new editor)

## Why

Tailoring copies a "base" CV and rewrites the copy for one vacancy. Today the base is
invisible:

- `/my/cvs` lists only tailored CVs. A base CV is never shown, so it cannot be opened,
  read, renamed or edited by hand.
- The only CV editor lives inside the tailoring workspace (`/tailor/<slug>`), which needs a
  vacancy. `/my/cvs/[id]` is a legacy redirect into that workspace.
- The base is chosen silently: `GetBaseCVByUser` (`internal/platform/db/queries/cvs.sql`)
  takes the most recently edited non-tailored CV. A user may already own several, but the
  only way to "switch" bases is to make the wanted one the newest edit, which nobody can
  discover.

The owner looked at Huntr's resume builder (`/track/resume-builder/home/base`) as the
reference: a list of base resumes, each with Edit, Tailor to Job, Download, Duplicate and
Delete, and every tailored resume built off a chosen base. That is the shape we want,
without copying Huntr's screens.

Success: a candidate targeting two kinds of role keeps two bases, edits each by hand, and
tailors a vacancy from the right one without thinking about which CV was edited last.

## Goals

1. **Base CVs are visible on `/my/cvs`.** A "Base CVs" block above the tailored list, one card
   per base CV (non-tailored, `is_tailored = false`): title, template, last edited, a small
   preview if cheap, and actions **Edit**, **Tailor to a job**, **PDF**, **Duplicate**,
   **Delete**.
2. **The existing tailoring workspace, in a BASE mode, edits the base CV — with the agent.** The
   owner's decision: no new editor UI. "Edit" on a base card opens the same workspace component
   (`web/src/lib/tailor/TailorWorkspace.svelte`, rendered by `/tailor/[slug]` for a vacancy and by
   `/tailor/base/[id]` for a base) bound to that CV with **no vacancy**. In base mode the
   vacancy-only parts are absent — the job, the match analysis and Job Match score, the ATS delta
   against the base, the cover letter and the autopilot run — and what stays is the CV preview, the
   section editor, template / typography / margins, the experience bank, the revision history with
   undo, and the agent chat. `/my/cvs/[id]` remains the address-of-a-CV redirect: a base goes to
   `/tailor/base/<id>`, a tailored copy to `/tailor/<slug>?cv=<id>` as before.
   - **The agent runs under a new `base` assistant preset** (migration 0232 widens the
     `assistant_sessions.preset` CHECK): a session bound to the CV and no vacancy. It registers the
     tailoring CV tools minus the vacancy ones — `cv_get`, `cv_edit` (same path policy, same
     evidence gate, same experience-bank provenance), `request_confirmation`,
     `check_evidence_fidelity`, `cv_page_count`, plus the usual bank tools — under a prompt that
     says it is editing the candidate's general CV, not aiming it at an employer. It is a preset of
     its own rather than `tailor` with a NULL job because the tailor prompt and tool set are all
     about a vacancy and `tailor` sessions are metered as tailoring sessions.
   - **The conversation is opened by `POST /me/cvs/:id/base-session`** (cookie; reuses the one
     already bound to the CV). It is deliberately not behind the paid-AI gate and spends no
     tailoring session: a free account still opens the workspace and edits by hand. The gate
     guards each turn — every assistant message already passes `mw.paidAI(FeatureAssistant)` — so a
     free account's first message meets the same upgrade state the tailoring workspace shows. Turns
     of a base session are metered per turn under the assistant allowance, like a chat; the
     tailoring session allowance is not spent.
   - **Agent edits are candidate-directed**, so they set `hand_edited` like the candidate's own
     edits (see "Hand edits are never overwritten automatically").
3. **Duplicate.** The owner's chosen way to create a second base (decided over a Huntr-style
   "create new from profile / PDF / LinkedIn" flow, which is deferred). Duplicating a base
   makes a new base with the same document, template and appearance, titled
   "`<title>` (copy)", and opens it in the workspace's base mode.
4. **Choose the base when tailoring — only when there is a choice.**
   - One base (or none): nothing changes, no extra step. None still seeds a base from the
     extracted résumé, as today.
   - Two or more: the pre-flight confirm-tailor dialog (`confirmTailorDialog.svelte.ts` /
     `ConfirmTailorDialog.svelte`) shows a picker of bases, preselected to the default base
     (below). The choice is sent with the tailor request.
   - "Tailor to a job" on a base card starts the existing tailor-for-a-job intake
     (`JdIntakeDialog`) with that base already chosen.
   - The `/my/cvs` header's "Tailor for a job" opens the same intake; with two or more bases
     and none preselected, the intake shows the same picker (my call — otherwise the most
     obvious entry point would skip the choice).
   - The tailor request is made when `/tailor/[slug]` loads and again on reload, so the
     chosen base travels in that URL (e.g. `?base=<id>`), not in client memory.
5. **A tailored CV remembers its base.** New nullable column `cvs.base_cv_id` (uuid,
   references `cvs(id)` `ON DELETE SET NULL`), written when the tailored copy is created.
   The ATS delta (`cv_ats_delta.go`) and `StartTailorSession` report the tailored CV's own
   base instead of "whatever is newest now". When it is NULL (copies made before this
   change, or the base was deleted) they fall back to the default base — today's behaviour.

## The default base

One rule, used everywhere a single base is needed without asking (preselection in the
picker, a tailor request without `base_cv_id`, auto-apply, reseed, the profile and header
heal readers that call `BaseCV` today):

1. the base that the user's most recently created tailored CV was made from (still
   existing, still a base), else
2. the most recently edited base (today's rule).

Rationale: once bases are edited by hand in the new editor, "last edited" would flip the
default every time the candidate fixes a typo in the other base. "Last used for tailoring"
is what "remember my choice" means. This rule replaces the query behind `BaseCV`, so every
existing caller follows it without a second copy of the rule.

Two details keep the default from moving on its own (builder-check answers, my call):

- **The migration backfills `base_cv_id`** on every existing tailored CV with the base that
  is the user's default when the migration runs (today's rule: newest-edited non-tailored
  CV). Without it every existing copy is NULL, the fallback decides, and the first hand
  edit or Duplicate in the new editor would silently become the default.
- **Step 2 of the rule becomes "the oldest created base"**, not the newest edited. It only
  applies to a user with no tailored CV pointing at a live base; for them, too, a
  Duplicate or an edit must not steal the default.

## Hand edits are never overwritten automatically (owner's decision)

Three paths rewrite "the base" wholesale from the résumé seed today, all through `BaseCV`:
`reseedBaseIfStaleVsUpload` (on every `POST /me/cvs/tailor` and every auto-apply tailor,
`auto_apply_tailor.go`), `ReseedCV` (refreshes the tailored copy and then the base) and
`ReseedBaseCV` (`POST /me/cvs/base/reseed`). They were harmless while nobody could edit a
base. With the editor they would silently erase hand edits after a new CV upload.

The owner chose: **a base the candidate has edited by hand is never rewritten
automatically.**

- The automatic stale-vs-upload refresh skips a base that carries a hand edit. How "edited
  by hand" is recorded is the builder's call — `cv_revisions` already records who made each
  change (the candidate's editor vs the system seed), which may be enough without a column.
- `ReseedCV` refreshes the tailored copy only; it no longer touches any base.
- The workspace's History tab offers **"Reset changes"** in base mode ("Update from my CV"), which calls the explicit reseed for THE
  OPEN base (the explicit action may overwrite; the candidate asked). `ReseedBaseCV` keeps
  working for its existing caller (the profile experience page) and targets the default
  base. A confirm step says the hand edits on this base will be replaced.
- A base never edited by hand keeps today's automatic refresh, so nothing changes for the
  candidate who never opens the editor.

## API

- `POST /api/v1/me/cvs/tailor` takes an optional `base_cv_id`. It must be one of the
  caller's own non-tailored CVs, else 404 (foreign/missing) or 409 (a tailored CV).
  Absent → default base. The response already carries `base_cv_id`. This is used by the
  web, the CLI and MCP alike, and it is additive. The endpoint is documented only in the
  internal spec (`web/src/lib/docs/api-spec.ts` and the generated internal OpenAPI JSON),
  so the new field goes there; publishing `/me/cvs/*` in the public
  `web/static/openapi.yaml` is out of scope (my call).
- Listing bases needs a read that does not change `GET /me/cvs`, whose items carry
  `job_slug`/`job_title`/`job_company` and are consumed by the CLI and the legacy redirect.
  A separate list of bases (with an `is_default` flag on the default one) is the builder's
  call on shape and path.
- Duplicate is a server operation (copy the stored, sanitized document; never round-trip
  it through the client). Only a base may be duplicated; duplicating a tailored CV is out
  of scope.
- Editing and deleting use the existing `PUT /me/cvs/:id` and `DELETE /me/cvs/:id`; the agent's
  edits go through the same `cvedit` path. `POST /me/cvs/:id/base-session` opens a base's agent
  conversation (see goal 2).

## Non-goals

- Huntr's "new base from profile / upload / LinkedIn" form, target job title and
  experience-level fields on a base. Later, if wanted.
- Tailoring the same vacancy from two different bases. The one-tailored-copy-per-(user,
  job) rule (migration 0091) stays: a repeat tailor of a vacancy reopens its existing copy,
  whatever base is picked. The dialog should not pretend otherwise.
- A base picker for auto-apply. It uses the default base.
- A vacancy-less autopilot run or an ATS/score view for a base: there is nothing to measure it
  against. The base mode's agent is conversational only.

## Anti-goals (would make this a failure even if it works)

- Showing a picker to someone with one base. The owner was explicit: one base, no choice.
- Breaking the CLI or MCP: `GET /me/cvs` keeps its shape; `POST /me/cvs/tailor` keeps
  working without the new field.
- Editing a base quietly changing which base tailoring uses.
- Deleting a base that tailored copies came from breaking those copies. They keep working;
  their `base_cv_id` becomes NULL and the ATS delta falls back.

## Constraints

- Layering: everything here sits in `internal/candidate/cv` (store, rule) and
  `internal/api/handler` (routes) — no new package, so `blocks.go` is untouched.
- SQL lives in `internal/platform/db/queries/cvs.sql`, regenerated with `make sqlc`.
- The migration is a new file after `0230_*`. `cvs` is small, but `pnpm check:sql`
  (squawk) still lints it; follow its guidance for adding a foreign key.
- Every user-visible string goes through the page/component `messages.ts` files with every
  locale the neighbouring messages carry.
- `knip` gates unused exports: whatever the legacy redirect page no longer uses goes.
- Before pushing: `gofmt`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./...`;
  integration tests under `internal/api/handler` cover the new routes (they use unexported
  constructors like `newCVHandlers`).

## Left to the builder

- The exact layout of the base cards and the editor page (reuse the workspace's left-pane
  and preview arrangement; match the existing `/my/cvs` card styling).
- Whether the editor autosaves or has an explicit save, following what the tailoring
  workspace's editor already does.
- How the picker looks inside the confirm dialog (a radio list or a select).
- Path and shape of the bases list and the duplicate route.
- The empty state when a user has no base yet (offer to create one from their CV/profile
  via the existing seed path, as tailoring would).
