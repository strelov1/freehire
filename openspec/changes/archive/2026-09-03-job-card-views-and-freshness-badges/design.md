## Context

The card (`web/src/lib/components/JobRow.svelte`) is the single source of truth
for how a job appears in every list — browse, search, company detail, tracking,
the assistant deck. It already renders a relative posting timestamp, a
reality/ghost chip, facet chips, credentials, country flags, skills, salary and a
client-computed profile-match bar. What it renders nothing of is attention or
freshness-as-a-signal.

Almost everything this change needs already exists:

- `jobs.view_count` — maintained offline by `cmd/rollup-views` from nginx access
  logs, exposed as `view_count` on the public job projection
  (`internal/job/jobview/jobview.go`), generated into
  `web/src/lib/generated/contracts.ts`.
- `web/src/lib/freshness.ts` — `freshnessBadges(postedAt, reality, appliedCount)`
  already computes the `New` / `Be an early applicant` pair, with three named
  thresholds, a reality gate, honest tooltips, and 12 tests. Its only caller is
  `JobView.svelte` (the job detail page).
- `formatCount` in `web/src/lib/activityChart.ts` — already abbreviates counts as
  `697K` / `3.4M`.
- `JobDocument` (`internal/search/search/document.go:41`) embeds `jobview.Job`, so
  every indexed document already holds `view_count` as a top-level numeric field.

So the work is mostly wiring, and the design decisions are about *not* building
second copies of things.

Constraints that shape it:

- The card is shared by two wire shapes. `Job` carries `view_count`,
  `applied_count` and `reality`. `Card` (the tracking/assistant projection)
  carries `posted_at` and nothing else from that list — so the card must degrade,
  not assume.
- `reality` is served by `/jobs/search` hits (it is on the index document, and
  backs the `reality.class` facet) but NOT by the `/jobs` list: `attachGhostToRows`
  computes reality locally and assigns only `views[i].Ghost`, never
  `views[i].Reality`.
- The header rail is narrow, shares its width with a truncating company name, and
  reserves a `pr-9` gutter for the save button.
- Meilisearch runs one serial task queue, and a settings update is a task on it.

## Goals / Non-Goals

**Goals:**

- Show `view_count` on the card without a second request, a new column, or a new
  index field.
- Bring the existing freshness badges to the card, computed by the existing rule.
- Add a `Most viewed` ordering with the smallest possible backend surface.
- Keep every threshold and every abbreviation rule in exactly one place.

**Non-Goals:**

- Changing any badge threshold, or the badge rule itself. `NEW_DAYS`,
  `EARLY_DAYS` and `EARLY_APPLIES` keep their current values and their reasoning.
- Changing `JobView.svelte`. The detail page renders exactly what it renders
  today.
- Any change to how `view_count` is *produced*. `cmd/rollup-views`, the nginx log
  format, the bot filter and the `Sec-Purpose` rule are untouched.
- A `view_count` filter (`min_views=`). Ordering answers "what is popular"; a
  filter would need a facet and a control nobody asked for.
- Backfill or migration of any kind.

## Decisions

### Reuse `freshnessBadges` rather than writing a card-specific rule

*Why:* it exists, it is tested, and it already solves a problem a fresh
implementation would have missed. Its reality gate catches the source that
rewrites its posting date on every crawl (`fake_freshness`) — without it the card
would print `New` on the oldest job in the catalogue. Its tooltips already name
what was counted rather than implying knowledge of the employer's inbox.

The stronger reason is consistency: the card and the detail page are two views of
one posting, and a threshold copied into the card would eventually drift from the
one in `freshness.ts`, so the same job would read `New` in the list and not on its
own page. One function, two callers.

*Alternative considered — a new `jobSignals.ts` keyed on `view_count < 25`
instead of the applied count.* This was the initial direction and was dropped on
finding the existing module. The critique behind it is real — `applied_count` is
near zero catalogue-wide, so it discriminates weakly, while `view_count` spans
zero to thousands. But `freshness.ts` already answers it: the threshold is
deliberately small (3) so the claim "survives being wrong by an order of
magnitude", and the tooltip states the basis. Switching the shared rule to
`view_count` would change the detail page too, rewrite half the existing tests,
and overturn a documented argument — for a signal that is better but not
obviously enough better to justify that. Retuning is cheap and reversible later;
shipping two disagreeing rules is not.

### The card requires the reality signal; the shared rule still does not

`freshnessBadges` lets the date stand alone when `reality` is absent — documented
as "the ordinary case for a job we have only just met". That is right on the
detail page, where the signal is always computed. It is wrong on a card, because
of where cards get their data: the browse feed comes from `/jobs/search` (hits
carry `reality`), but the `/jobs` list does not attach it, and the `Card`
projection has no such field at all. On those surfaces the gate would pass
blindly and the badges would appear on exactly the postings the gate exists to
suppress — on the surface people scan fastest and least critically.

*Decision:* add one exported wrapper in `freshness.ts` that returns nothing when
the signal is absent and delegates otherwise. The card calls the wrapper; the
detail page keeps calling `freshnessBadges`.

*Why a wrapper and not a guard in the template:* the rule "a card without the
signal shows no badges" is a rule, and a rule inside a Svelte template cannot be
tested. This is the same argument `facetModel.ts` makes for holding the sort
resolution ("in the component nothing could test it").

*Why not change `freshnessBadges` to require the signal outright:* that would
change the detail page's behaviour for a job with no signal, which is not what
this change is for, and would invalidate an existing tested case.

*Accepted consequence:* the `Card`-projection surfaces — the tracking board, the
saved and hidden lists, the assistant deck — show no badges. That projection has
no `reality` field at all, so it cannot support the claim honestly, and an absent
badge is a smaller cost than a false one.

The company detail page is NOT one of them, contrary to what this section said
before production verified it: `web/src/routes/companies/[slug]/+page.server.ts`
loads its jobs through `searchJobs` with a `company_slug` facet, so its rows are
search hits and carry the signal like any other. The plain `/jobs` list, which
genuinely does not attach reality, has no caller in the SPA at all — it is an
API-only endpoint. So the rule bites exactly one family of surfaces, and it is
the `Card` shape rather than any particular page.

### `formatCount` moves to `utils.ts`

The card needs count abbreviation; `formatCount` already implements it, in
`activityChart.ts`.

*Why move it rather than import it where it is:* a card importing a chart module
for a string helper is a coupling that reads as an accident. `utils.ts` already
holds `timeAgo`, which the card already imports for the very same rail, so the
two rail helpers end up in one place. `activityChart.ts` imports it from the new
home.

*Why not a third copy:* there is already a second k-formatter in
`github.svelte.ts`. That one is not orphaned by this change so it is left alone,
but it is precisely the drift a third copy would extend.

### The eye glyph moves to the count; the personal marker becomes a check

The rail renders `Eye` for "you have viewed this". The count also wants an eye.

*Why swap rather than pick another icon for the count:* the eye is the
conventional glyph for a view count, and the count is the new, public,
information-bearing thing. The personal marker is a private annotation, and a
check reads as "seen by you" at least as well. Keeping the eye on the personal
marker would push the count onto a less legible glyph to protect an earlier
choice.

*Why not two eyes differentiated by fill or weight:* two same-shaped glyphs in
one narrow rail meaning different things is exactly the ambiguity worth an icon
change to avoid.

This is a visible change to existing behaviour, which is why it is recorded as a
`web-frontend` spec modification rather than slipped in as an implementation
detail.

### Thousands abbreviate on the card, not on the detail page

*Why:* the rail is narrow and shares it with a truncating company name; `12403 ·
2d` crowds the name out. The detail page has room and keeps the exact figure, so
the precise number is always reachable.

### `view_count` gets no search-outbox plumbing

*Why:* the incremental push is gated on indexed content changing, and
`cmd/rollup-views` moves the counter without touching that content — so the
counter genuinely does drift between rebuilds. But the scheduled full rebuild
reads from Postgres every few hours, while the rollup that *produces* the counter
runs daily. The indexed figure is therefore never staler than the source figure,
and outbox plumbing would add write volume to a queue whose backlog has already
caused incidents, in exchange for no freshness.

*Alternative considered — have `cmd/rollup-views` enqueue the jobs it touched.*
Rejected on that arithmetic: it would enqueue a large fraction of the catalogue
daily to reach an ordering the rebuild already provides.

### `Most viewed` is always on offer

*Why:* it depends on neither query text (unlike `relevance`) nor a signed-in
profile (unlike `match`). Gating it would be inventing a precondition.

*Consequence handled deliberately:* the sort control renders only when it holds
more than one option, and `newest` was previously the only unconditional one.
With `views` also unconditional the control renders on every listing, including a
signed-out browse with no query — which shows no control today. That is an
intended improvement, but it makes an existing spec scenario false, so that
scenario is rewritten rather than left to contradict the new rule.

### The backend change is two allowlist entries that must land in order

`searchSortable` in `internal/api/handler/search.go` and `SortableAttributes` in
`internal/search/search/client.go`.

*Why the order is load-bearing:* `searchSortable` is what stops an unknown `sort`
from reaching Meilisearch. If the handler accepts `view_count` while the live
index has not declared it sortable, Meilisearch rejects the **whole query** and
this package's error mapping turns that into a 500 — a broken search, not an
ignored parameter. `internal/search/search/AGENTS.md` records this same
one-directional hazard twice already (for filterable attributes and for the match
embedder): settings first, binary second.

*How settings actually reach production:* there is no operator script for it.
`EnsureIndex` is called only from tests, and settings otherwise arrive when
`cmd/reindex` builds a fresh index and swaps it in. So either the rebuild carries
the setting, or it is patched by hand — and a hand patch must send the
**complete** `sortableAttributes` list, because Meilisearch replaces that setting
wholesale. Sending only `view_count` would silently drop `posted_at` and break
the feed's default ordering for every caller.

*Why this is a better position than the match sort was in:* declaring the
embedder did not retro-fill 1.36M documents, so that control shipped dark behind
a flag until a rebuild landed. Here the values are already in every document, so
the ordering is correct the moment the setting applies. No flag, no dark period.

`searchSort` already resolves `order` and defaults to `desc`, so no handler logic
changes — only the map.

`GET /api/v1/jobs` accepts no `sort` and is hardcoded `created_at DESC`. Since
the browse feed calls `/jobs/search`, nothing is needed there; adding a sort to
the DB list would build a second ordering path for no caller.

### The signal row's render guard has to widen

The row is conditional on there being a reality chip, a facet tag, a country or a
credential. A badge-earning posting with none of those would compute a badge and
have no row to draw it in.

## Risks / Trade-offs

**[The handler accepts `view_count` before the index declares it sortable]** →
Meilisearch rejects the entire query, breaking search rather than degrading the
sort. Mitigation: the setting is applied and *confirmed* before the handler
change rolls out; the ordering is recorded in the migration plan rather than
remembered.

**[A hand patch of `sortableAttributes` sends a partial list]** → `posted_at`
stops being sortable and the feed's default ordering breaks for everyone, which
looks nothing like the change that caused it. Mitigation: the patch sends all
five attributes, and the task is verified by re-reading the setting back, not by
a 200 on the patch.

**[The `SortableAttributes` update queues behind an in-flight rebuild]** → Meili
runs one serial task queue, so the settings task waits and the sort keeps
answering under the old settings — a deploy that appears to have done nothing.
Mitigation: check the queue is clear first; do not apply during a rebuild window.

**[A rebuild silently refuses below the free-disk floor]** → if the rebuild is
what carries the setting, it never applies, quietly. Mitigation: verify disk, and
confirm the setting is live by reading it back. As of 2026-09-03 a rebuild is in
flight with the floor temporarily lowered.

**[The badges never reach the `Card`-projection surfaces]** → the tracking board,
the saved and hidden lists and the assistant deck carry no reality signal, so
they show no badges. This is the chosen trade-off (an absent badge beats a false
one), and since those surfaces never had the badges, nothing regresses for users
— it only bounds where the feature reaches. Production confirmed the bound is
narrower than this document first claimed: the company page is search-backed and
does show them.

**[Sorting by views entrenches what is already popular]** → the ordering feeds
attention to postings that already have it, and a good fresh posting can never
top it. Accepted: it is one ordering of four, never the default, and the
freshness badges are the counterweight inside every other ordering.

**[`view_count` counts readers while the badge speaks of applicants]** → the two
signals now sit in the same card and could be conflated. Mitigation: the badge's
tooltip states its own basis, and the badge is not derived from the count, so
neither number is presented as evidence for the other.

**[The sort control now appears where it never did]** → signed-out visitors
browsing with no query previously saw no control. Accepted as an improvement (the
ordering was unreachable for them), and recorded as a spec change so it reads as
a decision rather than a regression.

**[Two badges plus existing chips can push the signal row to a second line on a
phone]** → the row already wraps by design and the badges are short, so this is a
layout cost, not a break. Worth checking on a narrow viewport during
verification.

## Migration Plan

No data migration. Deploy order is load-bearing:

**There are two deploys, not three.** `deploy/bin/release.sh` builds `cmd/server`
and the SvelteKit app in the same blue/green flip, so the handler's new
`searchSortable` entry and the visible `Most viewed` option go live together.
Treating them as separately shippable would be planning against a release path
that does not exist.

1. Make `view_count` sortable on the **live** index, and confirm it by reading
   `sortableAttributes` back and seeing all five. Either let a rebuild carry it
   (a rebuild builds settings from the binary) or patch by hand with the complete
   list. Not during a rebuild window — Meili's queue is serial, so the task would
   wait behind it and the setting would appear not to have applied. A 200 on the
   patch means the task was accepted, not that it ran.
2. Only once step 1 is confirmed, run the release. It carries the handler entry
   and the card together.

The card additions (count, glyph, badges) depend on neither step and cannot fail
this way; they are simply carried by the same release.

**No feature flag.** `sort=match` shipped dark behind `PUBLIC_MATCH_SORT` because
declaring its embedder did not retro-fill 1.36M documents — the ordering was
genuinely thin until a rebuild landed, a window nothing could shorten. There is no
equivalent window here: the counter is already on every document, so the ordering
is correct the instant the setting applies. The only exposure is step 1 not having
been done, which is a single verifiable read rather than a condition to wait out.
A permanent runtime flag to guard a one-time ordering would be complexity that
outlives its reason.

Rollback: remove the `views` option from the client vocabulary and release. The URL
parameter degrades on its own — an unrecognised `sort` resolves to the contextual
default — so a stale link or saved search never errors. Leaving the sortable
attribute declared is harmless.

## Open Questions

None. The badge thresholds are deliberately unchanged; retuning them is a
separate, one-line decision that can be taken after the badges are live on the
card and their frequency can be observed.
