## 1. The mapping

- [ ] 1.1 Add `collectionForSkill(skill)` to `web/src/lib/collections.ts`, directly beside
      `collectionForCategory` and shaped like it: return `{ slug, title }` for the collection
      whose `params` hold exactly one key, `skills`, whose value is the given slug as a
      **string**. Return `undefined` for everything else, including an empty input.
      The comment states both guards and why each exists — the single-key one excludes the
      four category-pinning slug collisions, the scalar one excludes a future list-valued pin
      whose OR semantics are not this skill's set.
- [ ] 1.2 Unit tests in `web/src/lib/collections.test.ts`, in the shape of the existing
      `collectionForCategory` block: `python` and `kotlin` resolve; all four collisions
      (`data-science`, `data-engineering`, `devops`, `machine-learning`) return `undefined`;
      a described skill with no collection (`sinatra`) returns `undefined`; `''` returns
      `undefined`. The four collisions are the point of the test, not padding — a slug-match
      implementation passes every other case.

## 2. Glossary → collection

- [ ] 2.1 In `web/src/routes/skills/[slug]/+page.svelte`, derive `jobsHref` from
      `collectionForSkill(data.slug)` when it resolves and keep the existing
      `/jobs?skills=<slug>` otherwise. Both the count sentence and "See all N →" already read
      `jobsHref`, so neither markup block changes.
- [ ] 2.2 Check the two `eslint-disable svelte/no-navigation-without-resolve` comments that sit
      on those links. They justify a query-only `/jobs` URL with no route segment to resolve;
      a `/collections/[slug]` URL DOES have one. Resolve it properly for the collection branch
      rather than carrying a suppression that no longer describes the code.

## 3. Collection → glossary

- [ ] 3.1 In `web/src/routes/collections/[slug]/+page.server.ts`, compute a `glossaryLink`
      beside `marketLink`: the pinned skill when `collection.params` holds exactly one key and
      it is a scalar `skills`, else `null`. It needs no description lookup — see design.md.
- [ ] 3.2 Render it in `+page.svelte` next to the `marketLink` block, in that link's voice:
      `What is Kotlin? — the glossary entry →`.

## 4. Spec

- [ ] 4.1 `skill-glossary` delta: extend "The glossary is reachable and crawlable" so the rule
      it already states about published links covers the postings link — the glossary does not
      publish a link to a URL that canonicalizes elsewhere when an indexable page for the same
      set exists.
- [ ] 4.2 `job-collections` delta: a filter collection pinning exactly one skill links to that
      skill's glossary entry.

## 5. Verification

- [ ] 5.1 `pnpm -C web lint`, `pnpm -C web check`, `pnpm -C web test`.
- [ ] 5.2 Render `/skills/kotlin` and `/collections/kotlin` locally (or read the SSR HTML) and
      confirm the links now exist in both directions and resolve to 200.
- [ ] 5.3 Confirm `/skills/data-science` still links to `/jobs?skills=data-science` and NOT to
      `/collections/data-science` — the case a slug match would get wrong.
- [ ] 5.4 Confirm `/skills/sinatra` (no collection) is unchanged.
