# Design

## Decision 1: the placeholder list is keys, not strings

`web/src/lib/placeholderRoles.ts` holds a `Category[]` drawn from the generated
`CATEGORY_VALUES` contract; labels resolve through `categoryLabel()`.

A hand-written `['QA', 'DevOps']` would be a second copy of a vocabulary the backend
already owns, and it would rot silently — a category renamed or retired upstream leaves
the placeholder confidently offering a value the feed can no longer filter on. Typing the
array as `Category[]` turns that into a build failure, which is the same guard
`CATEGORY_LABELS` already uses (`satisfies Record<Category, string>`), for the same
reason. AGENTS.md is explicit about the cost of the alternative: one vocabulary existed in
four copies, they disagreed, and every resulting miss was silent.

The module composes whole placeholder strings (`Search jobs — e.g. Backend`) rather than
exposing bare labels and letting the component interpolate. The phrasing is then testable
and lives in one place, and `HeaderSearch` stays a box that renders what it is handed —
it does not learn that one of its callers is about jobs.

Order: `backend → frontend → devops → qa → data_science → product`. Busiest first, because
under `prefers-reduced-motion` the first entry is the only one anyone sees.

## Decision 2: three guards, all required

**Rotation stops at the first interaction and never restarts.** Text moving under the
cursor while someone composes a query is the failure mode an animated placeholder invites.
Once stopped the placeholder freezes on the entry currently shown rather than reverting to
a static string — reverting would be a visible jump at the exact moment the visitor's
attention is on the field.

**`prefers-reduced-motion` disables it.** Renders the first entry and holds. Fifteen
components in `web/` already honour this; the detection follows the existing
`window.matchMedia('(prefers-reduced-motion: reduce)')` pattern
(`AuthBrandPanel.svelte`, `SourcesField.svelte`) rather than inventing a switch.

**The accessible name stays still.** `HeaderSearch.svelte` passes one prop to both
`placeholder` and `aria-label`, so a rotating placeholder would rotate the field's *name*
— a screen reader would announce the input as `Search jobs — e.g. QA`. The prop splits
into `placeholder` (the example, may move) and `label` (the honest, static name). `label`
is required rather than defaulting to `placeholder`: a default would silently reinstate
the trap at the next call site, and there are only two.

The fade is a `::placeholder` colour transition. An absolutely-positioned overlay span
would give finer control and would also put a second element inside the box's flex row,
where the real input, the location prefix, the clear button and the filters trigger
already negotiate width — a layout risk taken for a cosmetic gain.

## Decision 3: saved-on-the-board is a read-side change

`JobBoard.svelte`'s `build()` drops rows whose `columnOf` returns null, and saved-only
rows are exactly those. Mapping them to `preparing` is one line and needs no migration, no
dual write, and no backfill: it applies retroactively to every bookmark, and the rollback
is the same line.

The alternative — having the new button write `stage='preparing'` explicitly — would leave
old bookmarks and new ones behaving differently forever, and would put the same fact in two
places (a `saved_at` and a stage that merely restates it).

**Accepted risk.** The board fetches at most 500 rows and saved rows count toward that cap.
`JobBoard.svelte`'s own comment already records this. Rendering the rows makes an existing
consequence visible rather than creating it: a user with hundreds of bookmarks may push
older applications outside the window. That comment is updated in place — a second note
elsewhere would be a second answer to one question. The escape hatch it already names is a
server-side board-minus-saved filter, and that stays the fix if this bites.

## Decision 4: the completeness card lives on /my/tracking

`/my` is a 308 redirect to `/my/tracking`, not a page. The card goes where people already
land; inventing an account home to host it would be building infrastructure ahead of a
need, and the redirect exists precisely because there was nothing to put there.

Five steps, all readable from stores that already exist — **no new endpoint**:

| Step | Source |
|---|---|
| CV uploaded | `resumeStore` |
| Specialization and seniority | `profileStore` |
| Skills | `profileStore` |
| Location and work mode | `profileStore` |
| A saved search with an alert | `api.savedSearches()` |

The first four are the onboarding wizard's own steps (`cv`, `confirm`, `skills`,
`location`), so the card measures the same thing the funnel asked for and cannot drift
from it. The fifth is what makes the product act on its own — jobs arriving without a
visit — and is the one step the reviewer did not name. It is included because a
completeness meter that ends at a filled-in form measures paperwork, not activation.

The calculation is a pure module (`web/src/lib/accountCompleteness.ts`) taking the three
inputs and returning the step list with a done flag each, so it is unit-testable in plain
Node like `facetModel.ts` and `suggestions.ts` beside it.

The dot sits on the avatar, beside the notification bell. Two signals compete for that
corner; the bell wins on urgency, so the completeness dot never carries a count and is the
quieter of the two.

## Testing

Pure modules carry unit tests, matching how `facetModel.ts` and `suggestions.ts` are
covered:

- `placeholderRoles.ts` — the list is non-empty and unique, every key resolves to a
  non-empty label, and every composed string carries its label.
- `accountCompleteness.ts` — each step's predicate, a fully-populated account reporting
  complete, and an empty account reporting every step open.
- `columnOf` — a saved row with no stage lands in `preparing`; an explicit stage still
  wins.

The rotation timer, the button and the card are component-level and are verified in the
browser rather than with tests that would only restate the implementation.

## Out of scope

- `PUBLIC_MATCH_SORT` on production — an ops check, recorded so it is not lost.
- Any change to what `save` means on the server. Saving still writes `saved_at` and
  nothing else.
- The companies search placeholder. It is not a role box.
