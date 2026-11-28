## 1. The mapping

- [x] 1.1 Add `collectionForSkill(skill)` to `web/src/lib/collections.ts`, directly beside
      `collectionForCategory` and shaped like it: return `{ slug, title }` for the collection
      whose `params` hold exactly one key, `skills`, whose value is the given slug as a
      **string**. Return `undefined` for everything else, including an empty input.
      The comment states both guards and why each exists — the single-key one excludes the
      four category-pinning slug collisions, the scalar one excludes a future list-valued pin
      whose OR semantics are not this skill's set.
- [x] 1.2 Unit tests in `web/src/lib/collections.test.ts`, in the shape of the existing
      `collectionForCategory` block: `python` and `kotlin` resolve; all four collisions
      (`data-science`, `data-engineering`, `devops`, `machine-learning`) return `undefined`;
      a described skill with no collection (`sinatra`) returns `undefined`; `''` returns
      `undefined`. The four collisions are the point of the test, not padding — a slug-match
      implementation passes every other case.

## 2. Glossary → collection

- [x] 2.1 In `web/src/routes/skills/[slug]/+page.svelte`, derive `jobsHref` from
      `collectionForSkill(data.slug)` when it resolves and keep the existing
      `/jobs?skills=<slug>` otherwise. Both the count sentence and "See all N →" already read
      `jobsHref`, so neither markup block changes.
- [x] 2.2 Check the two `eslint-disable svelte/no-navigation-without-resolve` comments that sit
      on those links. They justify a query-only `/jobs` URL with no route segment to resolve;
      a `/collections/[slug]` URL DOES have one. Resolve it properly for the collection branch
      rather than carrying a suppression that no longer describes the code.

## 3. Collection → glossary

- [x] 3.1 In `web/src/routes/collections/[slug]/+page.server.ts`, compute a `glossaryLink`
      beside `marketLink`. It needs no description lookup — see design.md.
      Changed during implementation: the condition is NOT read from `collection.params`, as
      this task first said. By the time params reach the load they have been through
      `scopeParams`, which flattens a list-valued pin to its FIRST value — so a two-skill feed
      would arrive indistinguishable from a one-skill feed and earn a link to a glossary entry
      it is not about. `marketLink` beside it has the same latent hole, unbitten only because
      no category is list-valued. The condition therefore lives in `skillForCollection`, the
      inverse of `collectionForSkill`, which reads the raw registry where the difference
      survives.
- [x] 3.2 Render it in `+page.svelte` next to the `marketLink` block, in that link's voice:
      `What is Kotlin? — the glossary entry →`.

## 4. Spec

- [x] 4.1 `skill-glossary` delta: extend "The glossary is reachable and crawlable" so the rule
      it already states about published links covers the postings link — the glossary does not
      publish a link to a URL that canonicalizes elsewhere when an indexable page for the same
      set exists.
- [x] 4.2 `job-collections` delta: a filter collection pinning exactly one skill links to that
      skill's glossary entry.

## 5. Verification

- [x] 5.1 `pnpm -C web lint`, `pnpm -C web check`, `pnpm -C web test`.
- [x] 5.2 Render `/skills/kotlin` and `/collections/kotlin` locally and confirm the links exist
      in both directions. Done against a dev server proxying the prod API, checked in the SSR
      HTML and then in the live DOM: `/skills/kotlin` renders "9,374 open Kotlin jobs" and
      "See all 9,374 →" both pointing at `/collections/kotlin`, and `/collections/kotlin`
      renders "What is Kotlin? — the glossary entry →" pointing back. Screenshotted both;
      the glossary link sits under the description in the collection header, styled like the
      `marketLink` beside it, and the skill page is visually unchanged (only its hrefs moved).
- [x] 5.3 Confirm `/skills/data-science` still links to `/jobs?skills=data-science` and NOT to
      `/collections/data-science` — the case a slug match would get wrong. Confirmed, and
      `/collections/devops` renders no glossary link for the mirror-image reason.
- [x] 5.4 Confirm `/skills/sinatra` (no collection) is unchanged. Confirmed — still
      `/jobs?skills=sinatra`.

      5.1 results: 1,805 web unit tests pass across 155 files, `pnpm run lint` exits 0 with
      only the pre-existing warning backlog, and `pnpm run check` reports 0 errors.
