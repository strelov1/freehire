# internal/cvedit

The only path that writes a stored CV. Every change becomes a revision: the operations that
made it, the inverses that would undo it, who made it, and through which entry point.

## The shape of an edit

Four structural operations — `set`, `insert`, `remove`, `move` — over typed paths. They replace
eight named ops (`add_bullet`, `set_skill_group`, …) that reached about a third of the
document; these reach all of it, education and certifications and typography included.

A `Path` is validated by **reflection over the json tags** of `State` (`title`, `template_id`,
and the document's own fields, flat). Not against a list kept beside them: a hand-maintained
vocabulary drifts, and `set_stack` once went missing from a tool schema for a release. `Paths()`
enumerates the shapes from the same reflection, and that is what the model's schema is
generated from — so schema and struct cannot disagree.

The paths address `State`, which is the document plus the two columns the candidate edits
beside it. `agent_session_id`, `autopilot_report` and the copy's identity are deliberately
outside it: nobody edited them, and they would read as nonsense in a feed of edits.

## Inverses

`Apply` computes them *during* application, because that is the only moment the previous value
is still in hand. `Apply` is all-or-nothing: a refused batch leaves the state untouched, so a
rejected edit is never a partial one.

Which inverse a revision STORES depends on whether the sanitizer moved anything:

- **It did not** (the overwhelming majority) — `Apply`'s inverse is stored. It reverses each
  operation in kind, so undoing a reorder is one `move` back.
- **It did** — `Diff(sanitized-after, before)` is stored instead. The sanitizer drops empty
  entries and whitespace-only bullets, every index after such a drop shifts, and an inverse
  computed a moment earlier removes the wrong element. That cost a real experience entry in the
  test that found it.

Storing the diff unconditionally was the first attempt, and it broke reorders: the differ has no
`move` in its vocabulary, so a reorder's inverse came back as field-by-field rewrites of every
entry the move touched — and applying those overwrote whatever had been edited since, which is
exactly the promise undo makes and must not break.

An absent list and an empty one are the same stored state (the json tags omit both). Both
`Apply` and `Diff` treat them as one; keeping them distinct had the differ reporting operations
between two documents that are identical once saved.

## The differ

`Diff(old, new)` derives operations from two states, which is what makes whole-document `PUT`
an input *format* rather than a second write path. Equal-length lists compare in place;
differing lengths go through LCS, and an adjacent `remove`+`insert` of similar values collapses
into a `set` — otherwise rewording one bullet reads as "deleted a bullet, added a bullet". A
collapsed pair of entries recurses into their fields rather than swapping the entry whole.

The property every test asserts first: applying `Diff(a, b)` to `a` produces `b` exactly,
indices included.

## Coalescing, and why the inverse is not touched

Autosave fires 800 ms after a keystroke. A revision absorbs a follow-on edit when the actor, the
origin, the batch and the paths match, it is younger than a minute, and **both sides are `set`**
— the operations are then replaced (each save carries the place's current value, so the newest
batch already describes the burst) while the **inverse is left alone**, still leading back to the
state before the first keystroke.

Three conditions each stop a way of losing an edit:

- Appending instead of replacing makes the revision stop matching the shape of the next save, so
  a burst breaks into two entries on the third keystroke.
- Folding anything but `set` loses edits outright: two `insert`s at the same position are two
  additions, and keeping only the second leaves the first recorded nowhere and undoable by
  nothing.
- Ignoring the batch files a second agent turn's first edit under the previous run, and "undo
  the run" then misses it.

## Undo

The inverse is applied to the *current* state as a NEW revision naming what it reversed. Later
edits survive; the log is never rewritten, so an undo can itself be undone and the feed keeps
describing what actually happened. When the inverse no longer applies — the place it would
restore is gone — the attempt fails with `ErrCannotUndo`; that cannot be known without trying,
which is why the control is offered and the failure explained.

`RevertBatch` undoes a run: newest first, since a run's later edits sit on top of its earlier
ones. It is what retired `cvs.autopilot_undo`.

Neither the policy nor the evidence gate applies to an undo. An inverse restores text the
candidate already had on the page; asking it to cite evidence would refuse the undo of the very
edit the gate let through.

## Policy and the evidence gate

Access control is a table (`DefaultPolicy`), not a side effect of a small vocabulary. The agent
is denied `header.full_name`, `header.email`, `header.phone`, `header.links`, `title` and
`template_id`; the candidate is denied nothing.

**A denial reaches in both directions.** Downwards is obvious — denying `header.links` denies
`header.links[0]`. Upwards is the one that bites: `header` is not inside `header.email`, but
writing a whole `header` object replaces every contact identifier at once, and the addressable
vocabulary published to the model offers the container alongside the leaf. A rule that only
looked downwards was a rule the model could step over, and it did in the test that found it.

The gate is keyed on what is NOT a claim. `presentationShapes` is the **exemption** list — the
title, the template, style and margins, the header — the places that say how the CV looks and
what it is called, and nothing about the candidate's career. **Everything else is a claim by
default**: the summary and bullets, the stack line and skill groups, but also education,
certifications, entry roles, languages — and any field added to the document later, until
someone exempts it. The list is inverted on purpose. The earlier version named the places that
DO carry a claim — summary, bullets, stack, skills — so a degree nobody earned, a certification
nobody holds and a job title nobody had all landed uncited. They are the larger lie: a recruiter
checks a diploma, not a bullet's phrasing.

It applies to the **agent only**. The rule is that a model's inference must not reach the page;
the candidate writing about their own career is the source the bank exists to record.

The same reach applies: writing `experience[0]` writes the bullets inside it, so an operation is
gated when its shape is a claim shape **or a claim shape sits inside it**. Matching exactly
reopened the hole one level up.

`EvidenceID` sits on the operation, and one uncited operation refuses the whole batch —
otherwise a claim could ride in among valid ones.

## Titles

`Describe` renders a batch as the line the feed shows, from the operations plus the document, so
an address reads as "a bullet in Senior Engineer, Acme" rather than as a path. It is generated
on the server and never supplied by a model: the description is rendered as the application's
own words, and model-authored text presented that way is what the assistant's untrusted-content
boundary exists to keep out. An agent's `note` travels separately and is attributed to it.

## The transaction

`Repository.Edit` takes `GetCVForEdit … FOR UPDATE` and runs the whole commit inside it, so the
document and the revision describing it are written together or not at all — and two agent turns
arriving at once are serialised rather than interleaved.

The policy and the gate run BEFORE it opens. They are questions about the batch, not about the
document, and the gate reads the experience bank on its own connection: asking it while holding
the row lock borrows a second connection from a ten-connection pool for the length of a commit,
which is the shape a pool-exhaustion deadlock takes under load.

`Revert` checks that the revision belongs to the CV in the path, and `GetCVRevision` is scoped by
both. A revision id names an entry in one history; reading it through another CV of the same
owner would undo the wrong document.

`base_version` is journalling, not locking. The differ always compares against the state just
read, and an operation addressing a path that has since disappeared is refused when applied.

The feed is capped at 100 revisions per CV, trimmed in the same transaction as the insert: this is
an aid to the candidate's current work, not an archive, and each row carries two operation
documents on the table behind every CV page.

## Bullet ceiling

`cv.MaxBullets` (default 20, override with `CV_MAX_BULLETS`) is enforced by `Sanitize`,
which keeps the first N and drops the rest. Both `Commit` (an ops batch — cv_edit, template
picks) and `CommitDocument` (a whole-document save — the editor's PUT autosave, Reset from
résumé) refuse that class of loss up front (`ErrListCap` / `bullet_cap`) so neither an
agent's insert into a full list nor a pasted/seeded document that is itself over the cap can
look like a successful write while content vanishes. The guard is on by default;
`Editor.SetRefuseListCap(false)` (env `CV_EDIT_ALLOW_BULLET_TRUNCATION=true` on the server)
restores the old sanitize-and-drop behaviour without a code change, for both paths.

`CommitDocument` checks the RAW incoming document, before its own pre-diff `Sanitize()` call
— that Sanitize existed first (so the diff is against what will actually be stored) and would
otherwise erase the overflow before the guard ever saw it, making the refuse a no-op for
every whole-document save. Get this ordering backwards again and the guard silently stops
protecting PUT /me/cvs/:id and Reset from résumé while still working for cv_edit —
`TestCommitDocumentRefusesAWholeDocumentSaveOverTheCap` and
`TestUpdateCV_RefusesAWholeDocumentSaveOverTheBulletCap` pin it.

Sibling Sanitize `limit()`s — experience/education/skills/languages/projects/certifications
counts, skill items, links — still drop trailing entries silently. Extending refuse to those
lists is a follow-up if an incident appears; it is intentionally out of scope for the bullet
fix.
