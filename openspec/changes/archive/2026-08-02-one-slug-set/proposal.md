## Why

Catalogue **#38**: `savedJobs`, `viewedJobs` and `dismissedJobs` are three copies of one class
differing only in which endpoint they call.

They already shared the load-once/reset scaffolding through `UserResource<T>`. What stayed
duplicated is the payload half — a `SvelteSet` in `$state`, `has`, `mark`, sometimes `unmark`, and
the two hooks that swap the set on load and drop it on reset. Thirty lines, three times, differing
by one method call.

## What Changes

- `SlugSet` extends `UserResource<string[]>` and owns the set. A subclass supplies `load()` and
  nothing else — each of the three shrinks to five lines.
- `unmark` lives on the base rather than in the two subclasses that had it: `viewedJobs` simply
  had no caller for it, and an absent caller is not a different rule. It stays unreachable from
  outside `viewedJobs` because no `unmarkViewed` is exported.

## Also checked, and already done

**Catalogue #36** — "the single source of display labels is forked" — is **already closed by S1**
(`unify-facet-display-labels`, #1393). There is now one `titleCase`, one `categoryLabel` and one
`RELOCATION_LABELS` in `labels.ts`, imported by both `enrichment.ts` and `facets.ts`, with a test
pinning `not_supported → 'Not supported'`. Verified rather than assumed: the catalogue's numbering
and the shortlist's ids do not line up, so overlap has to be checked in the code.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change; `tasks.md` is the real artifact and the change
archives with `--skip-specs`.

## Impact

- `web/src/lib/userResource.svelte.ts`, `savedJobs.svelte.ts`, `viewedJobs.svelte.ts`,
  `dismissedJobs.svelte.ts`.
- **No unit test, and the reason is not laziness.** `SlugSet` is all runes, and this repo's own
  precedent says so out loud: `paginated.svelte.test.ts` carries the comment "the reactive
  Paginator can't be instantiated here — its `$state` fields need a Svelte runtime this test env
  doesn't provide". The collapse is verified by reading the three bodies side by side before
  merging them, and by `pnpm run check` staying at its baseline.
