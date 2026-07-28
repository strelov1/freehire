## Context

`cvs.id` is a bigint identity. It is exposed in `/my/cvs/<id>`, in the tailoring
workspace's `?cv=<id>`, across nine `/api/v1/me/cvs/:id/*` endpoints, and in two
published clients — the `freehire` CLI (`cv get|edit|render|context <id>`) and the
`freehire-mcp` server (`z.number().int()`).

Two tables reference it: `referral_requests.cv_id` (the CV attached to a referral
request, `ON DELETE SET NULL`) and `assistant_sessions.cv_id` (the CV a tailoring
conversation is bound to, `ON DELETE CASCADE`).

Production holds 44 CVs, so the data volume is irrelevant; the cost of this change
is entirely in the surfaces that name a CV.

The assistant's own session ids were made random while their table was unshipped
(`replace-assistant-runtime`). This applies the same reasoning to the one
user-owned resource still addressed by a counting number — and the most sensitive
one, since a CV is a résumé.

## Goals / Non-Goals

**Goals:**

- A CV is addressed by an unguessable id everywhere: API paths, web routes, the
  tailoring bootstrap's response, and both published clients.
- One identifier, not two. The bigint is gone rather than shadowed by a public
  alias, so no code path can accidentally accept or emit the old form.
- A malformed id is reported as missing, keeping "not a CV" and "not yours" one
  answer.

**Non-Goals:**

- Preserving the numeric form for a transition period. See the decision below.
- Changing ownership enforcement, response shapes beyond the id's type, or any
  tailoring behaviour.
- Re-addressing anything else. Jobs already use a public slug; companies use a
  slug; the assistant already uses a UUID.

## Decisions

### Swap the primary key rather than adding a public alias

The alternative — keep the bigint PK and add a `public_id uuid` addressed
externally — preserves compact indexes and foreign keys. It was rejected because
it leaves two identifiers for one row: every query, handler and test then has to
know which one it holds, and the failure mode is silent (a numeric id that still
resolves is exactly the enumeration we are removing). With 44 rows and two
referencing tables, the index-size argument is theoretical.

### Break the published clients rather than accept both forms

A dual-accept period would keep the numeric id working for as long as anyone runs
an old CLI — which is to say, the enumerable address stays live and the change
buys nothing until the last client updates. A defence that can be bypassed by
sending the older format is not a defence.

Breaking is also the safer failure: an un-updated client sends a number, the
backend cannot parse it as a UUID, and the request is refused as not found. There
is no id it can send that resolves to somebody else's CV.

Release order is backend first, then the two clients. Between the two, `cv`
commands fail with "not found" — visible, immediate, and fixed by an update.

### One transactional migration, converting the referencing columns with it

The new ids are generated in the same statement that adds the column, then the two
referencing tables are backfilled through the old key before it is dropped:

```
ADD cvs.new_id uuid DEFAULT gen_random_uuid()
ADD referral_requests.new_cv_id / assistant_sessions.new_cv_id
UPDATE both FROM cvs via the OLD bigint
DROP the old foreign keys and columns, RENAME the new ones
DROP the cvs primary key and the bigint id, RENAME new_id, re-add the key
RE-ADD both foreign keys against the uuid
```

All of it in one transaction, so a failure anywhere leaves the schema untouched.
The old numeric ids are not retained: nothing outside the database stores one
except the clients being released with this change.

### The frontend treats a CV id as an opaque string

`web/src/lib/cv.ts` types it `string` rather than `number`. That is what stops a
route or a request builder from quietly coercing, and it matches how the chat
already treats session ids.

## Risks / Trade-offs

- **An old CLI or MCP breaks until updated.** → Accepted, and the point: it fails
  loudly with "not found" rather than resolving to the wrong CV. Both clients are
  released immediately after the backend.

- **A bookmarked `/my/cvs/5` stops working.** → It 404s. CV URLs are private
  working links, not shared or indexed content, and the CV list is one click away.

- **The migration rewrites a primary key with two foreign keys hanging off it.**
  → One transaction, and it is exercised on a real Postgres by the integration
  suite (`startPostgres` applies every migration in order) before it goes near
  production.

- **Two clients must be released in step.** → The npm publish for `freehire-mcp`
  is manual, so the sequence is deliberate rather than automatic. Documented in
  the tasks.

## Migration Plan

1. Apply the migration manually on production, ahead of the API that reads it.
2. Release the backend and the web app together (one repository, one deploy).
3. Release `freehire-cli` and publish `freehire-mcp` to npm.
4. Between 2 and 3, `cv` commands on an un-updated client answer "not found".

Rollback: the migration is not reversible without regenerating the old bigints,
which nothing stores. Rolling back means rolling back the whole change, so the
migration is applied only after the backend passes its integration suite against
a database built from these migrations.

## Open Questions

None. The compatibility question — break versus dual-accept — was settled above.
