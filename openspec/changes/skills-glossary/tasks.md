## 1. The dictionary

- [ ] 1.1 Add `internal/dict/skilltag/descriptions.tsv` (empty but for a header comment
      row) and `descriptions.go`: `//go:embed`, a parser, `Description(canonical) string`
      returning "" for an unknown or undescribed slug, and `Descriptions() map[string]string`.
- [ ] 1.2 Add `descriptions_test.go`: no orphan keys (every key is in `Canonicals()`), no
      duplicate keys, no blank description, no tab or newline inside a description, and
      `Description` returns "" for an undescribed canonical and for a non-canonical slug.
- [ ] 1.3 Add the `describedFloor` constant and the coverage test asserting
      `len(Descriptions()) >= describedFloor`, with the comment stating the constant only
      ever rises and names the task that deletes it (5.3).
- [ ] 1.4 Add `Aliases(canonical) []string` — the spellings the parser accepts for a
      canonical, drawn from every alias tier, deduplicated and sorted, with a test
      covering a canonical reachable only through an acronym.
- [ ] 1.5 Record the description side in `internal/dict/skilltag/AGENTS.md`: what the TSV
      is, why generation is offline, and the ratchet.

## 2. The generator

- [ ] 2.1 Add `cmd/gen-skill-descriptions`: read `Canonicals()`, subtract
      `Descriptions()`, order the remainder by open-posting count from
      `GET /jobs/facets?facets=skills`, take `--limit`, and print TSV rows to stdout.
- [ ] 2.2 Prompt each undescribed skill with its slug, display label and `Aliases`;
      constrain the model to one or two sentences of plain language, no marketing, no
      restating the label. Use the service LLM credential, never a user's.
- [ ] 2.3 Test the ordering and the subtraction against a stubbed facets response and a
      stubbed LLM, so a run with no network is still meaningful.
- [ ] 2.4 Document the worker in the root `CLAUDE.md` worker list: what it needs, that it
      prints rather than writes, and that its output is edited before merge.

## 3. Wave 1 — the first 100 descriptions

- [ ] 3.1 Run the generator for the 100 most common skills, review and edit every
      sentence, and commit them to `descriptions.tsv`.
- [ ] 3.2 Raise `describedFloor` to match and confirm the coverage test passes.

## 4. The SPA seam

- [ ] 4.1 Emit `web/src/lib/generated/skillDescriptions.ts` from `cmd/gen-contracts`
      (its own file, not `contracts.ts`), with a test asserting the shared contracts
      module carries no description text.
- [ ] 4.2 Add the lazy accessor in `web/src/lib/`: `skillDescription(slug)` backed by a
      memoised `await import(...)`, returning "" when absent.

## 5. Coverage endgame

- [ ] 5.1 Wave 2 — descriptions 101–300, reviewed and merged, floor raised.
- [ ] 5.2 Wave 3 — the tail, reviewed and merged, floor raised.
- [ ] 5.3 Delete `describedFloor` and replace the coverage test with the absolute rule:
      a canonical with no description fails the build, mirroring the label rule.

## 6. The reveal

- [ ] 6.1 Add touch activation to `design-system/src/tooltip.svelte`: a coarse-pointer
      `pointerdown` toggles, an outside `pointerdown` dismisses; hover, focus and Escape
      unchanged. Extend `tooltip.test.ts` for the new path and to prove the old ones did
      not move.
- [ ] 6.2 Add a `SkillChip` component in `web/src/lib/components/`: the chip as it renders
      today, wrapped in the tooltip only when a description exists, with the description
      and a "What is X? →" link to `/skills/<slug>`. No affordance when undescribed.
- [ ] 6.3 Use it in `JobView.svelte` and `JobRow.svelte`, fixing the raw-slug render
      (`{skill}` → `skillLabel(skill)`) in the same edit.

## 7. The glossary pages

- [ ] 7.1 Add `web/src/lib/skillGlossary.ts`: pure helpers — described-slug lookup, the
      `MIN_SKILL_OPEN = 25` postings gate, and neighbour selection — unit-tested without
      fetch or Svelte, following `roleLandings.ts`.
- [ ] 7.2 Add the `/skills/<slug>` route: 404 for an undescribed slug; otherwise label,
      description, accepted spellings, neighbours, and the postings block behind the gate.
      Server-rendered, with page metadata.
- [ ] 7.3 Add the `/skills` index listing every described skill, grouped alphabetically.
- [ ] 7.4 Add `sitemap-skills.xml` listing exactly the slugs the route serves, and
      register it in the sitemap index.
- [ ] 7.5 Add the glossary to the internal-linking surfaces that already list the
      product's pages (`sitemap-pages.xml` and wherever `/roles` is linked from).

## 8. Ship

- [ ] 8.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`.
- [ ] 8.2 `pnpm check:links`, `pnpm check:dead`, and the web/design-system lint and tests.
- [ ] 8.3 Regenerate contracts (`make` target / `go run ./cmd/gen-contracts`) and confirm
      the committed generated files are current.
