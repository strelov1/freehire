# Design

## The shape of an edit

Four structural operations replace eight named ones:

```go
type OpKind string // "set" | "insert" | "remove" | "move"

type Op struct {
    Kind       OpKind `json:"kind"`
    Path       Path   `json:"path"`
    Value      any    `json:"value,omitempty"` // set, insert
    To         *int   `json:"to,omitempty"`    // move: the element's new index
    EvidenceID string `json:"evidence_id,omitempty"`
}
```

`Path` is the canonical text address of one place in the edited state — `summary`,
`experience[3].bullets[1]`, `style.font_size`, `template_id`. It parses to a sequence of field
and index steps and is validated **by reflection over the json tags of the edited state**, not
against a hand-written list. A list that is maintained by hand drifts: `set_stack` went missing
from a tool schema for a release, and a model that cannot see an op cannot use it. Reflection
also yields the set of legal paths for free, which is what the model's schema is generated from,
so schema and struct cannot disagree.

### What the paths address

The root is not `Document` but the CV's editable state: `title`, `template_id`, and the
document's own fields flattened alongside them. The candidate changes the template and the title
from the same workspace as the text, "switched to the Sidebar template" belongs in the feed, and
it is worth being able to undo. One address space covers everything a revision can change.

Other columns of the `cvs` row are **not** editable state and produce no revision:
`agent_session_id` records which conversation is attached, `autopilot_report` records what a run
made of each requirement, and `is_tailored` / `job_id` are the copy's identity. None of them is
something the candidate edited, and none would read sensibly in a feed of edits. The queries that
write them stay as they are.

## Inverses

```go
func Apply(state State, ops []Op) (State, []Op, error)
```

Inverses are computed during application, while the previous value is still in hand:

| Operation | Inverse |
|---|---|
| `set(p, new)` | `set(p, old)` |
| `insert(p[i], v)` | `remove(p[i])` |
| `remove(p[i])` | `insert(p[i], removed)` |
| `move(p[i] → j)` | `move(p[j] → i)` |

Inverses accumulate in reverse order. `Apply` is all-or-nothing: a failure at any operation
returns the state untouched, so a rejected batch is never a partial edit.

## Revisions

```sql
CREATE TABLE cv_revisions (
  id           uuid PRIMARY KEY,
  cv_id        uuid NOT NULL REFERENCES cvs(id) ON DELETE CASCADE,
  user_id      bigint NOT NULL,
  actor        text NOT NULL,          -- candidate | agent | system
  origin       text NOT NULL,          -- editor | tailor_agent | cli | template | import
  batch_id     uuid,                   -- one agent turn / one autopilot run
  title        text NOT NULL,
  note         text,                   -- the agent's own words, optional
  ops          jsonb NOT NULL,
  inverse      jsonb NOT NULL,
  base_version timestamptz NOT NULL,
  reverts_id   uuid REFERENCES cv_revisions(id) ON DELETE SET NULL,
  reverted_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX ON cv_revisions (cv_id, created_at DESC);
```

`user_id` is denormalised so every read is owner-scoped in one predicate, matching how the rest
of the CV tables are queried.

`base_version` is journalling, not locking. Optimistic concurrency is unnecessary by
construction: the differ always compares against the state just read from the database, and an
agent's operations address paths, so a path that has since disappeared is rejected by `Apply`
itself.

**The feed is capped at 100 revisions per CV**, trimmed in the same transaction as the insert. A
revision log is a convenience, not an archive, and the rows carry two json documents each on the
user's hottest table.

### Coalescing

Autosave fires 800 ms after a keystroke, so typing a paragraph would produce a dozen revisions.
When the newest revision has the same actor, the same origin, touches exactly the same paths and
is younger than one minute, it is **amended** rather than followed: `ops` are replaced with the
fresh ones, `updated_at` moves, and `inverse` is **left alone** — it still leads back to the
state before the first keystroke. That is what makes undo mean something for typed text.

### Undo

Undo applies a revision's inverses to the *current* state as a new revision carrying
`reverts_id`, and stamps `reverted_at` on the original. The log is never rewritten: the feed
stays truthful, an undo can itself be undone, and there are no holes. Reverting a whole run is
reverting every revision of a `batch_id` in reverse order, which is what retires
`cvs.autopilot_undo`.

When an inverse no longer applies — the bullet it would restore text into has since been deleted
— the attempt fails with a 409 naming that reason. Whether an undo is still possible cannot be
known without trying, so the button is offered and the failure is explained.

## The single writer

```go
Commit(ctx, cvID, userID, Change) (Meta, Revision, error)
CommitDocument(ctx, cvID, userID, state State) (Meta, Revision, error)
Revert(ctx, cvID, userID, revisionID) (Meta, Revision, error)
RevertBatch(ctx, cvID, userID, batchID) (Meta, error)
History(ctx, cvID, userID, limit) ([]Revision, error)
```

The repository method that writes `cvs.data` is unexported. `Editor` is the only exported way to
reach it — the invariant is held by visibility rather than by review.

`CommitDocument` diffs and calls `Commit`. Whole-document `PUT` is therefore an input format,
not a second write path.

## Policy and the evidence gate

Access control stops being a side effect of a small vocabulary and becomes a table:

| Actor | Denied paths |
|---|---|
| `candidate` | none |
| `agent` | `header.full_name`, `header.email`, `header.phone`, `title`, `template_id` |
| `system` | none |

The actor is decided by the entry point and never read from the request.

The evidence gate moves from op names to paths. A `set` or `insert` into a path that **asserts
something about the candidate** must cite banked evidence whose provenance is publishable:

- `summary`
- `experience[*].summary`
- `experience[*].bullets[*]`
- `projects[*].bullets[*]`
- `experience[*].stack[*]`
- `skills[*].items[*]`

The last two are new. `bulletWritingOps` lists only `add_bullet` and `replace_bullet` today, so
an agent can write "Kubernetes" into a technology line or a skill group unevidenced while the
same claim as a bullet is refused. It is the same class of assertion in different syntax.

`remove` and `move` require nothing: they rearrange or delete what was already said.

`EvidenceID` sits on the operation, not the revision. In a batch each writing operation answers
for itself, and one unevidenced operation rejects the whole batch — otherwise a claim could ride
in among valid ones.

Every refusal names what the caller may do instead. For a model, the error message is the only
route to correcting itself inside the turn.

## The differ

`Diff(old, new State) []Op` is a pure function — no database, no model:

- scalars (`summary`, `title`, `style.*`, `margins.*`) compare directly into `set`;
- arrays of equal length compare pairwise into `set` at the differing positions. This covers the
  overwhelming majority of edits, because an editor changes text in place;
- arrays of differing length go through LCS on exact equality, yielding `insert` and `remove`;
  an adjacent `remove`+`insert` pair whose values are similar enough collapses into `set`, so
  "rewrote a bullet" does not read as "deleted one and added another".

Detecting reordering as `move` is deliberately omitted: nothing in the editor reorders anything
by mouse today, so the differ has no way to produce that shape. It is added when the feed starts
reading badly, not before.

## Titles

A revision's title is generated from its operations together with the document, so it reads as
prose rather than as a path: `experience[2].bullets[1]` becomes "Rewrote a bullet in Senior
Engineer, Acme"; several operations fold into "Edited 3 bullets in Acme" or "Changed typography".

It is generated by the server, never by the model. The title is rendered in the feed, and text
from a model rendered in the UI is exactly what the assistant's untrusted-content boundary
exists to keep out. An agent may attach `note` — its own reason for the edit — which the feed
renders as the agent's words, plainly attributed.

## Preview highlighting

A revision's paths are handed to `CvHtmlPreview`, which underlines the matching nodes. Hovering
a feed entry highlights its edits, clicking pins them, and the newest agent batch is highlighted
by default until the candidate dismisses it.

`CvHtmlPreview` currently filters entries and bullets for emptiness
(`doc.experience.filter(…)`, `e.bullets.filter(b => b.trim())`), which discards the document's
own indices: one empty bullet between two filled ones shifts the numbering, and a highlight for
`experience[2].bullets[1]` lands silently on the wrong line. Filtering must carry the original
index through instead of dropping it. This is a prerequisite, not a nicety — the same mismatch
would misplace any future click-to-edit affordance.

## Testing

- `Apply` → inverse → original, per operation kind and for mixed batches. This is the invariant
  the whole feature rests on.
- `Diff`: scalars, equal-length arrays, insertion, deletion, and the `remove`+`insert` collapse.
- `Path`: parsing, rejection of unknown fields and of out-of-range indices.
- Policy: the agent cannot write `header.email`; the candidate can.
- Evidence: a batch with one unevidenced writing operation is rejected whole; `stack` and
  `skills` are gated like bullets.
- `Editor`: coalescing leaves inverses untouched; undo creates a new revision; batch revert runs
  in reverse; the feed trims at 100.
- Web: `vitest` over the pure TypeScript that builds paths and matches them to preview nodes.
  Components have no runner — those are verified visually.

## Migration

Two migrations, in separate releases. The first creates `cv_revisions`. The second drops
`cvs.autopilot_undo`, and only after the deployed code has stopped reading and writing it —
`cvs` is small, but a column dropped while a running binary still selects it is a 42703 on every
CV read.

`cvs.autopilot_report` is untouched: it records what a run made of each *requirement*, which is
a different question from what it changed.
