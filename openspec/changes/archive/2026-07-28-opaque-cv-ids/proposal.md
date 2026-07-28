## Why

A CV is addressed by a sequential number — `/my/cvs/5`, `?cv=5`,
`GET /api/v1/me/cvs/5`. Access is confined by ownership on every operation, so
the number is not a capability: a stranger asking for someone else's CV gets a
404 indistinguishable from a missing one. Two things are still wrong with it.

A counting id publishes volume: a user who creates a CV and sees `5` knows the
platform has created five, and one created a month later measures the growth rate
between them. And it removes a layer of defence — if an owner check is ever
missed on a new CV endpoint, a walkable id turns that single mistake into bulk
extraction of other people's résumés, which is the most sensitive data freehire
holds. A random id does not replace the ownership check; it makes forgetting one
survivable.

The assistant's session ids were made random for exactly this reason while their
table was still unshipped (see `replace-assistant-runtime`). CVs are the same
class of resource and the remaining place where a sequential id is exposed.

## What Changes

- **BREAKING:** `cvs.id` becomes a random UUID. Every CV-addressing surface —
  the `/api/v1/me/cvs/:id` family, the tailoring bootstrap's `tailor_cv_id` and
  `base_cv_id`, `assistant_sessions.cv_id` — carries that UUID instead of a
  number. There is no dual-accept period: the numeric form stops working.
- **BREAKING:** the `freehire` CLI's `cv` commands (`cv get|edit|render|context`)
  take a UUID argument rather than an integer, and the `freehire-mcp` tools
  (`cv_get`, `cv_edit`, `cv_render`, `cv_context`) declare a string id rather
  than `z.number().int()`. Both are released alongside; an un-updated client
  fails with a clear "not found" rather than silently reading the wrong CV —
  there is no id it can send that resolves to anything.
- **Migration:** existing rows (44 on production at the time of writing) get a
  generated UUID, and the foreign keys that reference `cvs(id)` are rebuilt
  against it. The old numeric ids are not retained: nothing outside the database
  stores them except the two clients being released with this change.
- **Unchanged:** ownership enforcement, response shapes other than the id's type,
  and the tailoring flow's behaviour. This changes what a CV is called, not what
  it does.

## Capabilities

### New Capabilities
<!-- none: this changes how an existing capability addresses its resource -->

### Modified Capabilities
- `cv-builder`: a CV is addressed by an unguessable id rather than a sequential
  one, and a malformed id is reported as missing.
- `cv-tailoring`: the tailoring bootstrap returns UUID CV ids, and the CV tools
  and endpoints address CVs by them.

## Impact

**Backend:** migration adding the column, backfilling it, and swapping the
primary key plus every foreign key that references it; `internal/db/queries/cvs.sql`
and the generated code; `internal/cv`'s `Store` and its `Meta`/`Record` ids;
the CV handlers (`cv.go`, `cv_tailor.go`) and the assistant's CV tools;
`assistant_sessions.cv_id`.

**Frontend:** the CV id type in `web/src/lib/cv.ts` and `api.ts`, the
`/my/cvs/[id]` route, and the tailoring workspace's `?cv=` parameter.

**Other repositories (released together):** `freehire-cli` (`internal/cli/cv.go`
argument parsing, `internal/client/cv.go` request paths) and `freehire-mcp`
(`src/tools.ts` input schemas). Both are published, so the release order is:
backend first, then the two clients — an old client sends a number, which the new
backend rejects as not found rather than mis-resolving.

**Deploy:** the migration must be applied manually on production before the API
that reads it, as with every schema change here.
