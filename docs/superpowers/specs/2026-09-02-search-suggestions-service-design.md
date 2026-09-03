# Search suggestions: a dedicated index behind one `/suggest` endpoint

**Date:** 2026-09-02
**Status:** approved, not implemented

## The problem

Visitors open the homepage feed and do not know what to type into the search box.
Three observed symptoms, in the order they hurt:

1. **An empty box says nothing.** Focus the field and no suggestion appears at all.
   `suggestRoles` returns `[]` below two characters (`web/src/lib/roleSuggest.ts:36`),
   so there is no entry point into the catalogue.
2. **A typo returns nothing.** `backedn`, `desinger`, `devlper` produce zero rows.
   `roleSuggest.ts:46` drops every match that only the typo tolerance reached — a
   deliberate choice, because ranking that tier by vacancy count put Marketing
   Specialist (55,768, reached through its `growth hacker` alias) above Backend
   Engineer.
3. **The thing people actually type is not in any dictionary.** `java developer`
   and `nodejs developer` name no role: the role catalogue carries exactly nine
   language-bearing slugs, all of them React Native. Yet the free-text search
   answers both well — `q=java developer` returns 37,533 postings titled
   "Java Developer", `q=nodejs developer` returns 9,386 titled "NodeJS Developer".
   The data exists; nothing surfaces it as a suggestion.

A fourth complaint is about the box's behaviour rather than its content: every
keystroke is pushed into the list filter, so the feed refetches while the visitor
is still typing.

## What exists today

- The homepage `/` **is** the jobs feed, so it renders `HeaderListSearch`
  (`web/src/lib/components/TopBar.svelte:26`), not the `HeaderSearch` launcher.
  The role dropdown therefore already ships on the homepage; it is its behaviour
  that falls short, not its absence.
- `web/src/lib/roleSuggest.ts` is a pure client-side matcher over the generated
  role catalogue: 1,830 labels plus 1,532 aliases, no network. Behind a 120 ms
  debounce (`SUGGEST_DEBOUNCE_MS`) because a pass costs ~10 ms — `fuzzy.ts`
  allocates an `Int32Array` per word/token pair.
- Picking a suggestion applies the `role` facet; it does not rewrite the query
  text. That model is kept.
- Search queries are reported to PostHog only (`track('search')`,
  `JobsView.svelte:552`). Nothing durable records what visitors ask for.

## Why a title facet is not the answer

`title` sits in `SearchableAttributes` but not in `FilterableAttributes`
(`internal/search/search/client.go:573`). Promoting it would not work: a facet is
a bounded value dictionary, and distinct titles number in the millions.
`MaxValuesPerFacet` truncates the distribution, `SortFacetValuesBy: count` decides
what survives, and the index pays for a filterable attribute it can never serve
completely. Confirmed against production: `GET /jobs/facets?facets=title` answers
`unknown facet: title`, and the 26 facets that do exist are all closed vocabularies.

The titles must be **mined into a bounded dictionary offline**, not exposed as a
facet online.

## Why this replaces the dictionary matcher rather than extending it

The obvious smaller fix is to patch `roleSuggest.ts`: admit the typo tier as a
fallback, rank it by edit distance rather than vacancy count, add an empty-state
list, make the pass two-phase so the debounce can go. Every one of those is work
the suggestions index does natively and better — Meilisearch already does prefix
matching, typo tolerance and ranking, over a corpus we choose. Building both means
two matchers that will disagree.

So `roleSuggest.ts` is retired by stage 2, and the stage-1 work is confined to what
survives regardless: when the search runs, and what an empty box shows.

## Design

### The index

A new Meilisearch index `suggestions`, sibling to `jobs`. One document per
suggestion:

```
{
  id:       "title:java-developer",
  text:     "Java Developer",
  kind:     "title" | "role" | "skill" | "category" | "company",
  slug:     "java" | null,      // the facet value to apply; null for a free-text title
  jobs:     37533,              // open postings behind it
  searches: 0                   // times visitors asked for it (stage 3)
}
```

Tens of thousands of documents against the 8M in `jobs`. Its smallness is the
whole performance argument.

`kind` decides what a pick does:

| kind | pick applies |
|---|---|
| `title` | free text `q=` — there is no facet for "Java Developer" |
| `role` | `role=<slug>` |
| `skill` | `skills=<slug>` |
| `category` | `category=<slug>` |
| `company` | `company_slug=<slug>` |

Facets combine with AND, so picking `Java` then `Backend Engineer` narrows rather
than replaces.

Companies come from the same source the `/companies` list reads — measured on
production, `google` carries 3,187 open postings under that slug, with
`google-india`, `google-fiber` and `google-ireland` as separate employers beside
it. They are emitted above a minimum-postings floor, so the long tail of one-off
company slugs does not drown the dictionary.

### Progressive completion

The dropdown completes a **phrase**, not a word. Typing `senior` offers "Senior
Software Engineer"; continuing to `senior software engineer go` must offer
"Senior Software Engineer **Google**" rather than starting over. Verified end to
end on production: `q=senior software engineer` with `company_slug=google` returns
871 postings, so the composed filter is real, not a nicety.

The endpoint therefore parses the query in two parts:

1. **The recognised prefix.** Greedy longest-match, left to right, against the
   dictionary's normalised phrases. `senior software engineer` is consumed as one
   `role` part.
2. **The trailing fragment.** Whatever the prefix did not consume — `go` — is what
   gets completed.

Candidates for the fragment exclude the kinds the prefix already filled: a query
that has named a role is not offered a second role, and one that has named a
company is not offered a second company. Skills are the exception — several skills
narrow sensibly, so the kind stays open.

Each row renders as the whole phrase and, when picked, applies **every** part at
once: role `senior_software_engineer` plus `company_slug=google`. A suggestion is
a composed query, not a single facet value.

**Two mechanisms, one job each, deliberately not merged:**

- The **parse** needs exact recognition of phrases the visitor has fully typed, so
  it runs against an in-process normalised phrase set the API loads from the
  `suggestions` index at startup and refreshes on a ticker. No typo tolerance is
  wanted here: a mistyped phrase should not be silently consumed as recognised —
  it should fall through into the fragment.
- The **fragment lookup** needs typo tolerance and ranking, which is exactly what
  Meilisearch already does. It stays a query against the index.

This is also what makes `backedn` work without a second matcher: the typo lands in
the fragment, and the index forgives it.

### The builder — `cmd/build-suggestions`

A run-once-and-exit cron worker, the `worker.Main` / `worker.Bootstrap` shape of
`cmd/rollup-facets`. Needs `DATABASE_URL`, `MEILI_URL`, `MEILI_MASTER_KEY`.

1. Walk the catalogue's open postings, normalise each title (lowercase, collapse
   whitespace, cut at the first separator — `|`, `(`, `,`, `/`, an em dash, or a
   literal " at "), and count. Keep titles above a minimum-occurrence floor — a
   title carried by one posting is noise, not a suggestion.

   **Measured before committing to this design**, over a 2,000-title sample from
   the live catalogue: 1,251 distinct normalised titles (62.5%), but the 204 that
   occur twice or more already cover 47.6% of the sample. The distribution is
   concentrated enough that a floor bounds the dictionary to the tens of thousands
   the index sizing assumes. The head is exactly what a suggestion should be:
   `senior software engineer` (60), `software engineer` (43), `data engineer` (32),
   `full stack developer` (17).

   **A frequency floor is necessary but not sufficient.** The same sample puts
   bare `manager` at 44 and `director` at 18 — frequent, and useless as a
   suggestion, because they name no craft. Titles that reduce to a bare seniority
   word or a bare generic (the `vocab.SeniorityValues` surface forms, plus
   `manager`/`director`/`consultant` alone) are dropped regardless of count. The
   role and category dictionaries already carry those axes properly.
2. Add the dictionary suggestions: roles (`roletag.Catalog`), skills
   (`skilltag`), categories (`vocab.CategoryValues`), each with the live facet
   count from `search.FacetCounts` — the same source `cmd/rollup-facets` reads, so
   the figures match the filters.
3. Join the query frequencies from `search_queries` (stage 3; zero until then).
4. Write with the swap the `jobs` rebuild already uses (`search.Rebuild`), so a
   reader never sees a half-built index.

The de-duplication rule, from measurement: a bare-category role and its category
are the **same rows**. Role `devops` counts 53,250 and category `devops` counts
53,251; role `data_analytics` 77,367 against category 77,375. Emitting both puts
one filter in the dropdown twice, which is the confusion this feature exists to
remove. **A category is emitted only when no role shares its slug**; the role wins,
because "DevOps Engineer" names a job and "DevOps" names a department.

### The endpoint — `GET /api/v1/suggest?q=`

One Meilisearch query against `suggestions`, returning at most 10 rows in the
standard list shape (`{"data": [...], "meta": {...}}`). Ranking: `searches`
descending, then `jobs` descending, then shorter text first. Typo tolerance and
prefix matching are Meilisearch's defaults — the same ones that already let
`q=nodejs` return NodeJS postings mid-word.

Empty `q` returns the empty-state set: the categories in `CATEGORY_GROUP_ORDER`
order (`web/src/lib/filterSections.ts:19`) — Engineering, Data & AI, Quality &
Security, and so on, with the consumer industries last. **Not the top values by
count**: measured on production those are Management (266,883), Sales (179,993)
and Support (127,110), which read as a different website to a visitor who came for
engineering work.

**Rate limiting is part of this endpoint, not an afterthought.** One request per
keystroke is 10-20x the request volume of the current search box. `/suggest` gets
its own bucket, keyed the way the public read limiter is keyed today — the prior
incident where `c.IP()` returned empty and put the whole site in one bucket is the
failure mode to avoid.

### The frequency table — `search_queries`

Migration `0123`. Columns: normalised query text (primary key), a count, and
`last_seen`. Written by the search handler on every request that carries `q=`,
upserted. No `user_id` and no session — the table records what the catalogue is
asked for, not who asked.

The normalisation is the same function the builder uses on titles, so a mined
title and a typed query land on the same key.

### The client

`HeaderListSearch.svelte` calls `/api/v1/suggest` with a short debounce (~80 ms)
and a request token, the stale-response guard `HeaderSearch.svelte` already uses.
The empty-state rows are fetched once on focus and cached for the session, so the
box is populated before the first keystroke.

**Rows carry a mark, as the launcher's already do.** `HeaderSearch.svelte` renders
each job and company row with `EntityLogo` fed by `companyLogoUrl(name)`
(`web/src/lib/logo.ts`), and a `company` suggestion must look the same — the
recognisable mark is what makes "Google" scannable at a glance. Reuse those two,
do not write a second logo path. The other kinds carry a kind glyph rather than a
logo: a `title` or `skill` names no employer.

A `company` row also names its posting count, which is what separates Google
(3,187) from Google India (19) in the same list.

**The dropdown has three sections, in this order:**

| Section | Source | Cap |
|---|---|---|
| completions | `/api/v1/suggest` | 5 |
| jobs | the existing jobs search, as the launcher calls it | 5 |
| companies | the existing companies list | 3 |

The jobs and companies sections are the launcher's, unchanged: `HeaderSearch.svelte`
already fetches both per keystroke and renders them with logos. Typing `google`
must show Google's actual postings, not only the phrase "Google" — that is what
the visitor came for.

This matters *more* now than it did before, not less: with the search no longer
running as you type, the feed below is stale mid-query, and these rows are the only
live evidence that the query is finding anything.

The load is not new in kind — the launcher already issues two requests per
keystroke and guards them with a stale-response token. It is new on the homepage,
which previously issued one. Budget for it in the `/suggest` rate-limit bucket
decision above.

Search runs on **Enter or on picking a row**. Typing no longer pushes into the
list filter, so the feed stops refetching mid-word. This is a behaviour change for
anyone who ignores the dropdown today and watches the list update as they type;
it is the change that was explicitly asked for.

### Package placement

`internal/search/suggest` — block `search`, layer 6. It reads `dict` (role, skill
and category vocabularies) and `platform` (db, the Meili client), both below it.
The package must be added to the `search` block's list in
`internal/platform/arch/layering/blocks.go:97`, or the layering guard fails.

## Staging

| Stage | Scope | Ships |
|---|---|---|
| 1 | Enter-to-search; empty box shows categories in curated order; the homepage dropdown gains the launcher's job and company sections with their logos | Frontend only, all of it reusing code that already exists. Removes the cheapest confusions with no backend risk. |
| 2 | `suggestions` index, `cmd/build-suggestions`, `/api/v1/suggest`, progressive completion, the client switched onto it; `roleSuggest.ts` retired | Closes "java developer", "nodejs developer", and "senior software engineer go" → Google. The bulk of the work. |
| 3 | `search_queries` table, write path, ranking by `searches` | Suggestions start learning from real demand. |

Stage 1 ships independently and touches no Go. Stage 2 subsumes stage 1's
empty-state source — the endpoint serves it instead of the client — so stage 1's
empty state is built against the same curated order to keep the swap invisible.

Progressive completion is stage 2 only. It needs the dictionary to parse against,
and a client-side approximation over the generated contracts would be a second
matcher of exactly the kind this design retires.

## Deployment notes

- Meilisearch runs **one serial task queue**. A suggestions build queues behind a
  running `jobs` rebuild. The index is small, so the wait is the cost, not the
  build — but the timer must not be scheduled on top of `freehire-reindexw`.
- The new index needs no change to the `jobs` index settings, so none of the
  "declare the filterable attribute before the binary flips" hazard applies.
- `cmd/build-suggestions` needs a systemd unit and timer in `deploy/`, and
  `deploy/` does not deploy itself — the unit must be copied to the host.
- Stage 3's migration must be applied before the binary that writes
  `search_queries` rolls out.

## Testing

- **Builder:** the normalisation and the minimum-occurrence floor over a fixture
  catalogue; the category-vs-role de-duplication rule (a category sharing a role's
  slug must not be emitted); the company floor.
- **The parse:** `senior software engineer go` splits into the role part and the
  fragment `go`; a mistyped phrase is NOT consumed as recognised but falls into the
  fragment; a query naming a role is offered no second role, while a query naming
  a skill is still offered more skills.
- **Endpoint:** the empty-`q` ordering follows `CATEGORY_GROUP_ORDER`; a typo
  query returns its intended target; the response shape matches the list contract.
- **Client:** typing does not refetch the list; Enter does; picking a `title` row
  sets `q=` while picking a `role` row sets the facet; picking a composed row
  applies **every** part, so "Senior Software Engineer Google" lands as role plus
  `company_slug` and not as one of the two.
- **The regression that motivates this:** `backedn` must return Backend Engineer.
  The previous attempt returned Marketing Specialist because it ranked the typo
  tier by vacancy count; assert the target, not merely that something came back.
- Integration tests are `//go:build integration` and do not compile under plain
  `go test ./...` — run `go vet -tags=integration ./...` before pushing.
