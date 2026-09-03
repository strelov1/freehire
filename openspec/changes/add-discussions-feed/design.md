# Design — global discussions feed

## The two problems, and why they are one change

Both live in the same listing code path.

**The cursor lies about the end of the list.** `ListThreads` sets
`meta.next_cursor` whenever `len(threads) > 0`. The page size is 30
(`community.Service`), so a subject with 1 thread returns 1 row AND a cursor,
the client believes another page exists, and "Load more" fetches nothing. The
handler cannot currently tell a full page from a last page because the page size
lives in the service's unexported config. `GetThread` has the identical bug for
replies.

**The surface has one entrance per subject.** Threads are addressed only through
their subject: `/jobs/<slug>/discussion`. Nothing lists threads across subjects,
so discovery requires already knowing which posting was discussed.

## Resolving a subject to a name

A thread stores `(subject_type, subject_ref)` where `subject_ref` is the
subject's public slug and there is **deliberately no FK** — the primitive stays
decoupled so a new `subject_type` needs no schema change (see
`migrations/0038_community_threads.sql`). A cross-subject feed therefore has
only slugs, and
`design-systems-lead-b2b-donut-studios-new-engen-inc-dk43ucun` is not a thing to
show a reader.

Resolution happens in SQL, in the listing query, with two LEFT JOINs:

```sql
LEFT JOIN jobs j      ON t.subject_type = 'job'     AND j.public_slug = t.subject_ref
LEFT JOIN companies c ON t.subject_type = 'company' AND c.slug        = t.subject_ref
```

Two properties make this safe. Both join keys are unique — `jobs_public_slug_key`
and `companies.slug` (the primary key) — so neither join can multiply a thread
row. And both are LEFT: content outlives its subject exactly as it outlives its
author. `cmd/prune` is a hard delete with no FK to cascade through, so a thread
can outlive its vacancy; an INNER JOIN would silently drop it from the feed,
which is the same class of bug the existing persona joins avoid.

Alternatives rejected:

- **Resolve on the client.** One request per row to `/jobs/<slug>`. N+1 against
  the API for a list whose whole purpose is to be scanned.
- **Denormalise a `subject_title` column onto `threads`.** Writes a copy that
  goes stale when a posting is re-titled, and buys nothing: the join is two
  unique-index lookups per row on a 30-row page.

## Why a separate read model

`community.Thread` is currently a faithful image of a `threads` row. Adding
`SubjectTitle`/`SubjectCompany` to it would make one type mean two things: the
subject-scoped listing would return it with those fields always empty, and
"the field is blank but shouldn't be" is the resulting bug. The feed gets
`community.ThreadWithSubject` — a `Thread` plus the resolved names — and the
existing listing keeps returning `Thread`.

`SubjectTitle` empty is the load-bearing signal for "subject is gone"; the wire
shape passes it through as an empty string and the client falls back to the slug.

## Wire shape

`GET /api/v1/threads/recent?cursor=<token>` — public, like the other read paths
(only persona handles are ever exposed). Registered **before** `/threads/:id`,
for the reason `count` already is: Fiber would otherwise parse `recent` as a
thread id.

The row extends `threadResponse` with `subject_title` and `subject_company`
(the latter only meaningful for `subject_type=job`), so a client can render
"Design System Lead / Design Ops · jito.dev" and link to the subject's existing
thread page. Standard envelope: `{"data": [...], "meta": {"next_cursor": ...}}`,
with the corrected cursor rule.

## The index

The feed orders by `(created_at DESC, id DESC)` over open threads only. The
existing `threads_subject_open_created_idx` leads with `(subject_type,
subject_ref)`, so it cannot serve an unfiltered scan. A new partial index
mirrors the same shape without the subject prefix. Partial on `status = 'open'`
for the same reason as the existing one: closed threads never enter the hot
index.

Ordering is by thread creation, not by last activity. A `last_activity_at`
column would have to be denormalised and maintained on every reply, and there
are zero replies in the catalogue — the two orders are currently identical. The
seam is the column; it is not added here.

## What the cursor rule still gets wrong

"Cursor only on a full page" fixes the case that prompted this — 1 row, cursor
emitted, "Load more" fetches nothing — but not its boundary. A listing holding
exactly `pageSize` rows returns a full page that is also the last page, so the
button appears once and then fetches an empty continuation.

Closing it properly means fetch-ahead: ask the repository for `pageSize + 1`,
trim to `pageSize`, and report whether the extra row existed. That changes what
the domain's three listing methods return (items plus a has-more flag), so it is
a change to the port, not to the handler. Left as the seam, and recorded here so
the residual is a known limit rather than a quiet lie: the frequency drops from
"any single-thread subject" to "a count that is an exact multiple of 30".

## Scope held back

- **No global "Start a topic."** A thread requires a subject, and a feed-level
  compose button would need a job/company picker — a different feature.
- **No thread URLs in the sitemap.** The section page is added to
  `sitemap-pages.xml`; a `sitemap-threads.xml` for 3 URLs is infrastructure
  ahead of need.
- **Feed is indexable per the product decision**, though it currently lists 3
  threads. Recorded here because thin content is a domain-level SEO risk, not a
  page-level one: if it costs ranking, the fix is `noindex` on this route, which
  is one line.
