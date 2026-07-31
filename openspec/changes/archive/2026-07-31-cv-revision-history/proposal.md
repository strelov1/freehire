## Why

A tailored CV is edited by two hands — the candidate's and the agent's — and neither can see
what the other did. What exists today is a single snapshot column, `cvs.autopilot_undo`, taken
once at the start of an unattended run. It answers exactly one question ("put the document back
the way the run found it") and answers it badly: `internal/cv/AGENTS.md` already records that two
runs started at once snapshot over each other, so undoing returns to the middle of the first.

Undoing one edit is impossible, and not for want of storage. Half the edits have no address at
all. `PUT /me/cvs/:id` carries the whole document, so the server cannot tell a font change from
three rewritten bullets — the editor's forms bind to one shared `doc` object and an effect ships
it wholesale 800 ms later. Without an address there is no line for a history feed, no highlight
in the preview, and no inverse to apply.

The patch vocabulary that does carry addresses covers a fraction of the document. Eight named
ops reach `summary`, bullets, skill groups, the stack line and one header field. Education,
projects, certifications, languages, margins, typography and contact details have no op at all
— they move only by whole-document `PUT`. The vocabulary is also the access control: the agent
cannot write the candidate's email because no op names it. That defence is real but accidental,
and it has a hole — `set_stack` and `set_skill_group` are not in `bulletWritingOps`, so an agent
can put "Kubernetes" on the CV as a technology or a skill without citing any evidence at all,
while the same claim as a bullet is refused.

## What Changes

- A new `internal/cvedit` package owns the only path that writes `cvs.data`. `Store.Update` and
  `Store.Patch` stop being public: every entry point — the editor's autosave, the template
  gallery, the CLI's `PATCH`, the assistant's `cv_edit`, seeding a tailored copy — goes through
  `Editor.Commit`.
- Edits are expressed as operations over typed paths (`set`, `insert`, `remove`, `move` against
  `experience[3].bullets[1]`, `style.font_size`, `template_id`). Four structural ops replace
  eight named ones and reach the whole document instead of a fraction of it. Paths are validated
  by reflection over `Document`'s json tags, so the vocabulary cannot drift from the struct.
- Applying operations yields their inverses, computed while the old value is still visible. Every
  commit is stored as a `cv_revisions` row with its operations, its inverses, its actor and its
  origin.
- Whole-document `PUT` remains as an input *format*, not a write path: a diff against the stored
  state derives the operations, and from there it is indistinguishable from an agent's commit.
- Undo is per-revision: the inverse is applied to the *current* document as a new revision, so
  later edits survive and the feed stays truthful. Undoing a whole agent run becomes undoing a
  `batch_id`, and `cvs.autopilot_undo` is retired.
- The vocabulary's accidental access control becomes an explicit path policy per actor, and the
  evidence gate moves from op names to the paths that carry a claim about the candidate —
  closing the stack/skills hole in the same move.
- The tailoring workspace gains a History tab: every edit with who made it, an undo button, and
  a preview that underlines what that edit touched.

## Capabilities

### New Capabilities
- `cv-edit-revisions`: every change to a stored CV is a recorded, addressable, individually
  reversible revision.

### Modified Capabilities
- `cv-builder`: the document is written only through committed revisions; `PATCH /me/cvs/:id`
  takes path operations instead of the named-op patch.
- `cv-tailoring`: the agent edits by path operations under an explicit policy, and the evidence
  gate covers every path that asserts something about the candidate.
- `tailor-autopilot`: a run's edits are grouped by batch and reverted as a batch, replacing the
  pre-run snapshot column.
- `tailor-workspace`: the workspace surfaces the revision feed and highlights a revision's edits
  in the live preview.

## Impact

`freehire-cli` calls `PATCH /me/cvs/:id` with the named-op body and breaks. It is updated in the
same wave; accepting both bodies would preserve exactly the legacy this change exists to remove.
