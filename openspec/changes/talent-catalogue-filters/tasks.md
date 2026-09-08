Every task runs the spec-driven-tdd micro-cycle: RED (a failing test first) → GREEN →
REFACTOR → simplify → review → only then `[x]`.

This change is STACKED on `talent-network-public-catalog`. Rebase or merge before the PR,
and **re-check the highest migration number on `origin/main` immediately before opening
it** — that branch had to renumber twice while it was open, and nothing gates it.

## 1. Counts over the snapshot

- [x] 1.1 `talentnetwork.Facets(q Query) Counts` — walk the snapshot the catalogue already
      holds, tallying each facet. Shape mirrors the jobs facet response
      (`{total, facets, stats}`) so the existing panes render it unchanged. No index, no
      new storage: the whole membership is already in memory.
- [x] 1.2 **A facet's own selection is removed before counting it**, while every other
      filter still applies. Test the trap directly: with one skill selected, other skills
      must still report non-zero — otherwise the pane silently becomes single-select and a
      visitor can never add a second value.
- [x] 1.3 A value nobody carries is ABSENT rather than reported as zero. A value carried by
      fewer members than the floor is present WITHOUT its number — the option stays usable
      and the number does not name somebody. Test both.
- [x] 1.4 The total the facets report equals the total the list reports for the same query.
      One test, both paths, because two counting paths over one snapshot is exactly the
      pair that drifts.

## 2. The endpoint

- [x] 2.1 `GET /api/v1/talent/facets` in `internal/api/handler/talent_catalog.go`:
      unauthenticated, same query vocabulary as the list, unread params reported in
      `meta.ignored_params` through the same `ignoredTalentParams`.
- [x] 2.2 Mount it on `talentCatalogLimiter`, the catalogue's OWN budget — a filtering
      visitor makes two requests where a reader makes one, and what bounds scraping the
      catalogue should bound scraping its shape. The existing AST guard
      (`publicReadLimiterFuncs`) already covers the register, so this needs no new list
      entry — confirm it still passes rather than assuming.
- [x] 2.3 Document it in `web/src/lib/docs/api-spec.ts` and regenerate `docs/API.md`.
      `web/static/openapi.yaml` stays untouched for the reason the list's own task records.

## 3. The store

- [ ] 3.1 Promote `web/src/lib/talentQuery.ts` to the pure model: add the facet mutators a
      `FacetStore` needs (toggle, add, remove, clear per facet), keep it free of `$app` so
      it stays unit-testable, keep its tests. Same split `companyFacetModel.ts` has under
      `CompanyFilterStore`.
- [ ] 3.2 `web/src/lib/talentFilters.ts`: `TalentFilterStore implements FacetStore` over
      `UrlSyncedState`. The catalogue has no `_exclude` and no AND/OR modes, so `cycle` and
      `pick` collapse to one include toggle and `toggleSign`/`setMatchAll` are inert —
      say so in a comment, as the company store does, rather than leaving a reader to
      wonder what they do.
- [ ] 3.3 Tests for the store's own behaviour: a toggle round-trips through the URL, a
      cleared facet leaves no empty parameter behind, and applying a query string replaces
      the whole state.

## 4. The modal

- [ ] 4.1 `TalentFilterModal.svelte` over `FilterModalShell`: a rail of Specialization,
      Skills, Seniority, Timezone, City, Experience. The shell owns the chrome and the
      deferred apply; this file owns only what is catalogue-specific.
- [ ] 4.2 Panes: `FacetSection` for the open vocabularies (skills, city) so they arrive
      searchable and counted; `ChipFacet` for the closed ones. Nothing new is drawn.
- [ ] 4.3 The staged surface commits to the URL on apply, so a narrowed catalogue stays a
      shareable link. Test that applying writes the URL and that the back button restores
      the previous filter.

## 5. The list surface

- [ ] 5.1 `web/src/routes/talent/+page.svelte` adopts `ListToolbar` (the filter button and
      the count), `FilterSummary` (removable chips saying what is narrowing the list) and
      `Pagination`, replacing the hand-rolled chip rows and the prev/next links.
- [ ] 5.2 `TalentCard` restyled against `JobRow`'s density. It carries less by
      construction — no employer, no salary, no posting age — so match the rhythm rather
      than filling the space.
- [ ] 5.3 The timezone notice survives the move: a timezone filter drops members whose zone
      is unknown, and the surface still says so. It was previously gated on a filter with
      no control, so nobody could reach it.

## 6. Verification

- [ ] 6.1 `gofmt -l .` prints nothing; `go build`, `go vet`, `go test ./...` and
      `go vet -tags=integration ./...` pass.
- [ ] 6.2 `pnpm --dir web check|lint|build` and the vitest suite pass;
      `pnpm check:dead` reports no new unused export.
- [ ] 6.3 Manual browser pass against the local stack (`seed-talent.sql` seeds three
      members with deliberately awkward data): search a skill, add a second, confirm the
      counts move and the result widens; narrow to one member and confirm the count is
      withheld while the option remains; check the URL survives a reload and the back
      button. **This is the pass that found the last change's real defect — do not skip
      it.**
