## Context

Specialization in the product is the category vocabulary `classify.Parse` resolves job titles into. Skills are the `skilltag` dictionary, also described by the skills glossary. Neither has a public page that lists what it holds, and there is no way to report a term it drops.

`dict-gap-reporting` stays a read-only curator report over enrichment output; it does not gain a table or a page.

## Goals / Non-Goals

**Goals:**

- A public `/dictionary` page that lists every recognised specialization and skill, with open-job counts.
- The same look and behaviour as the job filter modal, by reusing its components.
- A signed-in queue of missing terms, one live row per kind and folded term, counted rather than duplicated.

**Non-Goals:**

- Industries, geography, seniority, credits, a moderation screen, and a public list of the queue.
- Writing a dictionary entry from a row, or asking a model when a term is missing.
- Seeding the queue from enrichment or from zero-result searches.

## Decisions

### Reuse the filter modal components

The Specialization section is `CategoryPane` in `plain` (include-only) mode: grouped, collapsible chips from the static `CATEGORY_GROUPS`. The Skills section is `SearchSelect` with tech icons, fed from the skills glossary catalog (`loadSkillDescriptions`). Both components gain two optional props: a bindable search text (`query` on `CategoryPane`, `filter` on `SearchSelect`) and an `empty` snippet rendered in place of "Nothing found". The filter modal passes neither and is unchanged.

Selected chips live in a `StagedFilters` store. When any are picked, the page shows a "Show jobs" link to `/jobs` with those filters.

Alternative: page-specific list and search components. Rejected. They would copy the filter UI and drift from it.

### Lists from dictionaries, counts from search

The lists are what the dictionaries hold, so a term with no open jobs today still appears. Counts are one unfiltered `facetCounts(['category','skills'])` call, the numbers the filter modal shows. The call is best-effort: if search is down, the page still renders every term, without numbers. The page is cached at the edge for an hour.

### The server decides what is known

The page's "nothing found" is a text filter over labels and aliases. It can miss a spelling the dictionary resolves. So the write endpoint checks again:

- `specialization` is known when `classify.Parse(term).Category != ""`.
- `skill` is known when `skilltag.Canonicalize([]string{term})` is non-empty.

A known term is 409, and the page says it is already in the dictionary under another name.

### One table, plus a daily counter

`dictionary_gaps` holds the queue. `kind` is checked against `('specialization', 'skill')`. A partial unique index on `(kind, normalized_term)` where `status` is `pending` or `accepted` makes the live-row rule true in the database. `raw_term` and `submitted_by` stay the first writer's. `resolution_note` is for a human rejecting a row by SQL.

`dictionary_gap_daily (user_id, day, writes)` counts successful writes, including increments, in the same transaction. The 21st write of a UTC day returns 429 and rolls back.

The fold is `classify.FoldTerm`: trim, lower, `ё` to `е`, `&` to ` and `, other non-letters to a hyphen, trim hyphens. `classify.GapTerm` refuses an empty fold or a term over 80 characters (runes, so Cyrillic gets the same room). The write is one upsert, `INSERT ... ON CONFLICT` on the partial unique index, so the database decides insert versus increment and two first asks racing both count; a read-then-insert would fail the second on the index.

### The handler stays in the API

`internal/api/handler/dictionary.go` holds the one route. It calls `GapTerm`, the dictionary check, then sqlc. No new layer package. Cookie auth is the existing session middleware.

An anonymous click gets 401. The page puts the term in the URL (`?specialization=` or `?skill=`) and opens sign-in, so the visitor returns with the term already in the search box.

The queue has no public read. A successful write returns `{request_count, status}` for that term only, and the page shows "Asked N times".

## Risks / Trade-offs

- [The client filter and the server check can disagree] → The server is the authority. A term the page shows as missing but the dictionary resolves gets 409 and a clear message.
- [Signed-in users can still stuff the queue] → 20 writes per UTC day, and a rejected row's note survives a later ask. No public list to post on.
- [Counts depend on search] → Best-effort. The lists never do.

## Migration Plan

Add migration `0188_dictionary_gaps.sql`. It creates both tables and the partial unique index. No backfill. Rollback is dropping the two tables and removing the route; nothing in the catalogue reads them.

## Open Questions

None.
