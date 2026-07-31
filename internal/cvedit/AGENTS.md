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

`Apply` returns them. They are computed *during* application, because that is the only moment
the previous value is still in hand — afterwards, "what did this bullet say before" is a
question nobody can answer. They come back in reverse order, and `Apply` is all-or-nothing:
a refused batch leaves the state untouched, so a rejected edit is never a partial one.

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

Autosave fires 800 ms after a keystroke. A revision absorbs a follow-on edit when the actor,
the origin and the paths match and it is younger than a minute — the operations are **replaced**
(each save carries the place's current value, so the newest batch already describes the burst)
while the **inverse is left alone**, still leading back to the state before the first keystroke.
That is what makes undo mean something for typed text.

Appending instead of replacing is wrong twice: the revision stops matching the shape of the
next save, and a burst breaks into two entries on the third keystroke.

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

The gate is keyed on **paths that carry a claim about the candidate** — the summary, an entry's
summary, bullets, a project's bullets, a technology line, a skill group. The last two are new:
under the named vocabulary only two of eight ops required evidence, so a technology written into
a stack line arrived uncited while the same claim as a bullet was refused.

It applies to the **agent only**. The rule is that a model's inference must not reach the page;
the candidate writing about their own career is the source the bank exists to record.

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

`base_version` is journalling, not locking. The differ always compares against the state just
read, and an operation addressing a path that has since disappeared is refused when applied.

The feed is capped at 100 revisions per CV, trimmed by the same statement that inserts: this is
an aid to the candidate's current work, not an archive, and each row carries two operation
documents on the table behind every CV page.
