## Context

Three SPA modules render closed-vocabulary facet codes:

| module | surface | category label source | fallback for a code not in the map |
|---|---|---|---|
| `facets.ts` | filter panel, profile pickers | `labels.CATEGORY_LABELS` | local `humanize` — **Title Case Every Word** |
| `enrichment.ts` | job-detail facet rows | `labels.CATEGORY_LABELS` | local `humanize` — **Sentence case** |
| `insights.ts` | indexed `/insights` pages | **its own 35-entry map** | local title-case regex |

A fourth surface was missed in the first pass and found in review: `routes/open/+page.svelte`
labels its seniority and work-mode distributions with a private fallback and consults no
shared map at all, so the sitemap'd `/open` page prints "C level" and "Onsite" where the rest
of the app prints "C-level" and "On-site". Different vocabularies from the ones this change
set out to fix, same defect — so it is fixed here rather than left as a known divergence that
would falsify the first requirement on the day it is archived.

`labels.ts` states its rule in its opening comment: "only the codes whose label differs from
the title-cased fallback are listed here". That rule is coherent for exactly one fallback.
With two, every multi-word code the map omits is rendered two ways by construction — which is
also why `insights.ts` grew its own map rather than trusting the shared one.

So the shared map is not really the single source it claims to be: it is a partial override
table over two different defaults. The fix is to remove the ambiguity at its root rather than
to reconcile the two forks against each other.

Note that the seniority vocabulary does **not** have this problem: its codes are single words
(`intern`, `junior`, …) plus `c_level`, and single words title-case and sentence-case
identically. `insights.ts`'s `SENIORITY_LABELS` is therefore byte-equivalent to what the
shared map plus either fallback already produces, with one exception — the empty-string
"category-wide" band, which is an `/insights` concept and not a vocabulary value at all.

## Goals / Non-Goals

**Goals:**

- One code, one string, on all three surfaces.
- The rule that keeps them aligned is checked by the test suite, not asserted in a comment —
  the review theme this change is drawn from is "invariants held by prose".
- Settle the three wordings that currently differ, in one place.
- Leave the `/insights` publication logic, intro sentences and page set untouched.

**Non-Goals:**

- **Unifying the two fallback functions.** `enrichment.ts` deliberately sentence-cases; its
  doc comment says so, and other facets on the job page (domains, company types, English
  levels) still route through it. Changing that is a visual decision about facets this change
  does not touch, and it would be invisible in review if smuggled in here.
- **Making every label map exhaustive.** Only the category vocabulary suffers the two-fallback
  divergence, because only it has multi-word codes rendered on all three surfaces. Domains,
  regions, work modes and English levels stay override-only.
- **Touching the wire.** Filter values are the codes; nothing sent to the API changes.
- Restructuring `insights.ts` beyond removing the duplicated maps.

## Decisions

### Exhaustive category map, not a reconciled fallback

**Chosen:** make `CATEGORY_LABELS` cover all of `CATEGORY_VALUES`, so the fallback never fires
for a category and the two fallback styles become irrelevant to it.

**Alternative considered — make both surfaces call one `categoryLabel()` helper and leave the
map partial.** This also produces one string per code, and is a smaller diff. Rejected because
it leaves the trap armed: the map would still be a partial override table, so the *next*
vocabulary rendered on two surfaces repeats the bug, and nothing tells a maintainer which
codes are safe to omit. Exhaustiveness makes the map's contract stateable — and therefore
testable.

**Cost accepted:** `labels.ts` grows by ~25 entries that equal their title-cased fallback, in
apparent violation of its own "only the differing codes" rule. The rule is amended in the
comment for this one map, with the reason recorded in place.

### `satisfies Record<Category, string>` is the enforcement

`CATEGORY_VALUES` is generated from the Go vocabulary by `cmd/gen-contracts`, and
`contracts.ts` exports the matching `Category` union. Writing

```ts
export const CATEGORY_LABELS: Record<string, string> = { … } satisfies Record<Category, string>;
```

converts "keep this map in sync" from a comment into a compile error, in **both**
directions — verified against this repo's own tsc: a missing code fails with `TS1360`
naming the property, an extra one with `TS2353`. The declared type stays
`Record<string, string>`, so the `label(map, value)` helpers and the `options()` call sites
are untouched; `satisfies` constrains the literal without widening or narrowing the binding.

It runs in the required gate: `main`'s branch protection requires the `web` job, which runs
`pnpm run check` (`svelte-check`).

**Alternative considered — a unit test asserting `CATEGORY_VALUES ⊆ keys(CATEGORY_LABELS)`.**
That was the first implementation and it worked, but it is strictly weaker: it is
one-directional (a label left behind after a category is removed goes undetected), and it is
a bespoke assertion where the type system already expresses totality. Dropped in favour of
the one-line constraint.

**Alternative considered — declaring the map as `Record<Category, string>` outright.** Also
total, but it changes the exported binding's type, which would ripple into the helper
signatures and force a cast at the direct-index site. `satisfies` gets the check without the
ripple, which is exactly what it exists for.

### One title-case helper, owned by `labels.ts`

`facets.ts`'s `humanize` moves to `labels.ts` as the exported `titleCase` and is imported
back. Putting the fallback in the module that owns the maps keeps "how a label is produced"
in one place; leaving it in `facets.ts` would mean `labels.ts` importing from the filter panel
to define its own lookup.

`enrichment.ts`'s `humanize` is renamed `sentenceCase` in place. It is not moved and not
merged. The rename is in scope because the current name plus its doc comment ("Title-case an
unknown snake_case code (e.g. `data_engineering` → `Data engineering`)") is self-contradictory
and is a plausible reason the divergence went unnoticed.

### `categoryLabel` lives with the maps; `seniorityLabel` does not move

`categoryLabel` moves from `facets.ts` to `labels.ts`; its two importers
(`filterSections.ts`, `ProfileForm.svelte`) and the three `/insights` loaders point at the new
home. No re-export shim — a re-export chain would leave two importable paths for one function,
which is the shape of problem this change exists to remove.

`insights.ts` keeps exporting its own `seniorityLabel`, because it alone must answer for the
empty-string category-wide band. It delegates for every real code:

```
seniority === '' ? 'All levels' : (SENIORITY_LABELS[seniority] ?? titleCase(seniority))
```

Adding the `''` sentinel to the shared map instead was rejected: `''` is not a member of the
seniority vocabulary, and a shared map containing a non-vocabulary key would make the
coverage-test idea incoherent for seniority later.

### Relocation gets a named map

The filter panel's inline `{ not_supported: 'None', … }` and `enrichment.ts`'s module-level
`RELOCATION` become one `RELOCATION_LABELS` in `labels.ts`. Three entries; it is only worth
naming because two surfaces already spelled it differently.

## Risks / Trade-offs

- **A visible copy change ships on indexed pages** (~15 job-detail category rows gain a
  capital; two categories and one relocation value change wording). → Every change is a
  *convergence* onto a string the product already displays on another surface, and the
  `/insights` H1s and titles — the actual indexed text — keep their current strings except
  where the owner deliberately settled the wording. Nothing gains a spelling that appears
  nowhere today.
- **The exhaustive map will drift from the vocabulary.** → That is exactly what the coverage
  test prevents; it is the change's load-bearing artifact, not a nicety.
- **`labels.ts` grows and reads more repetitively.** → Accepted. A list that is boring to read
  and impossible to get silently wrong beats a clever partial map that renders two ways.
- **Someone re-adds a private map later.** → The spec requirement states the rule, and
  `labels.ts`'s comment records why the category map is exhaustive when its siblings are not.
- **`other` gets a label ("Other") although `/insights` never publishes it.** → Harmless: the
  filter panel and the job page can both receive `other` from the API today, and both already
  render it through their fallbacks.

## Migration Plan

None required. Display-only change in the SPA; no schema, no API, no index, no data
backfill. Rollback is a revert of the commit.

## Open Questions

None. The three wordings were settled by the product owner before implementation began.
