## Context

`internal/dict/skilltag` owns 863 canonical skills. Two things about them already ship:
the alias tables that resolve text to a canonical (`dictionaries.go`), and the display
labels that decide how a canonical is written for a reader (`labels.go`). Labels reach
the SPA through `cmd/gen-contracts`, which emits `SKILL_LABELS` into
`web/src/lib/generated/contracts.ts`; `web/src/lib/facets.ts` reads it.

This change adds a third side to the same vocabulary — what the skill *is* — and two
surfaces that reveal it. It touches four places that already have opinions:

- **`internal/dict` is dict-only.** Every dictionary in this block refuses to guess. A
  description programme that generated text at request time would break that rule for
  the first time, so generation has to be an offline, reviewed step.
- **`contracts.ts` is loaded eagerly, on every page.** It is 180 KB today. 863
  descriptions at ~130 characters each add roughly 110 KB — a 60% increase to a file
  every visitor downloads, to serve text most of them never open.
- **`/roles` set the honesty pattern for landing pages** (`web/src/lib/roleLandings.ts`):
  a block that would misdescribe the catalogue does not render, and the sitemap lists
  exactly what the route serves.
- **`design-system/src/tooltip.svelte` already exists** and is careful — hover, focus,
  Escape, a hide delay so the pointer can travel onto the content (WCAG 2.1 SC 1.4.13).
  It has no touch path, which is precisely the gap the issue names.

## Goals / Non-Goals

**Goals:**

- One curated sentence or two per canonical skill, reviewed by a human before merge.
- Coverage enforced by a test that cannot be quietly weakened, and that reaches "every
  canonical" without blocking the first wave on the last one.
- Reveal on the chip that works for pointer, keyboard and touch.
- A crawlable glossary page per described skill that is not a thin page.
- Zero cost to a reader who never opens a definition.

**Non-Goals:**

- Rewriting the skill vocabulary, or adding skills to it.
- i18n. English only; the descriptions are a dictionary, and a second language is a
  second dictionary with its own review burden.
- A long-form second body of text per skill. The glossary page is made substantial by
  facts the dictionary already holds (aliases, neighbours) and by live postings, not by
  a second LLM pass nobody would read.
- Any API change. No endpoint, no payload field, no migration.

## Decisions

### The text lives in a TSV beside the dictionary, not in a Go map

`internal/dict/skilltag/descriptions.tsv`, one `slug<TAB>description` row per line,
`//go:embed`-ed and parsed by a short `descriptions.go`.

*Why not a Go map:* 863 entries of prose in a `map[string]string` is the same data with
`"…": "…",` around every row. The value is a sentence, so the diff a reviewer reads is
the sentence either way — but in a TSV it is one line, unindented, with no quoting to get
wrong, and a wave's diff is a clean block of added lines. `internal/dict/location`
already ships its largest dictionary this way (`cities1000.tsv`), so this is the block's
existing shape rather than a new one.

*Cost:* a parser and its failure modes. Bounded by a test: a row with a tab or a newline
in the description, a duplicate key, or a blank description fails the build.

### Coverage is a ratchet, not an all-or-nothing gate

A `describedFloor` constant records how many canonicals are described. The test asserts
`len(descriptions) >= describedFloor` and that every key is a real canonical.

*Why not "every canonical must have one" from the start:* that rule is the endgame the
issue asks for, and it is right — but on day one it demands 863 reviewed sentences in
one PR, which means one reviewer skimming 863 sentences, which means the review is
theatre. The floor lets each wave be a PR a person can actually read, while making a
regression impossible.

*Why a floor and not a per-wave allowlist of slugs:* an allowlist is a second copy of the
vocabulary that drifts. A count cannot drift; it can only be wrong in one direction, and
the test catches that direction.

*The endgame is a task in this change, not a promise:* the last wave deletes the constant
and replaces the floor assertion with the absolute rule, so two rules never coexist.

### The generator is a run-once worker that prints, and never writes

`cmd/gen-skill-descriptions` reads `skilltag.Canonicals()`, subtracts what is already
described, orders the remainder by how many open postings carry the skill, takes the
next `--limit`, and prints TSV rows to stdout for a human to paste and edit.

*Frequency comes from the public facets endpoint* (`GET /jobs/facets?facets=skills`), not
from Postgres. A `GROUP BY unnest(skills)` over 8M rows is minutes of database work to
answer a question the search index answers for free and already publishes.

*It prints rather than writes* because the artifact is the reviewed text. A worker that
edited `descriptions.tsv` would make "reviewed" a promise rather than a property of how
the file got there.

*It needs `LLM_BASE_URL` / `LLM_API_KEY` / `LLM_MODEL`* and uses the service credential:
this is background work that belongs to nobody, matching the rule in `internal/ai/llmkey`.

### The SPA gets a second generated module, dynamically imported

`cmd/gen-contracts` emits `web/src/lib/generated/skillDescriptions.ts` beside
`contracts.ts`. A small accessor in `web/src/lib/` imports it with `await import(...)`
and memoises the result; SvelteKit code-splits it into its own chunk.

*Why not the API:* descriptions are ~863 rows and never per-job. Putting them in the job
payload ships the same sentence with every posting that names the skill; putting them
behind an endpoint adds a network round trip and a cache to a static asset the build
already knows.

*Why not `contracts.ts`:* the 110 KB above, paid by every visitor for a hover.

*Trade-off accepted:* the tooltip's first open is async. It shows the chip immediately and
the text when the chunk lands — one small fetch, then never again for that session.

### Touch support goes into the shared tooltip, not into a skill-specific copy

`design-system/src/tooltip.svelte` gains a pointer-type-aware activation: a `pointerdown`
from a coarse pointer toggles it, and an outside `pointerdown` dismisses. Hover and focus
behave exactly as now.

*Why change the shared component:* every other consumer has the same hole. A skill-only
tooltip would be a second component whose behaviour drifts, and the design system is the
place this repo puts interaction rules.

*Risk:* the change is visible to existing consumers. Contained by the component's own
tests (`tooltip.test.ts`) plus new ones for the touch path, and by the fact that mouse
and keyboard paths are untouched.

### The chip keeps its filter link; the tooltip carries the glossary link

A reader who clicks "Go" on a posting wants Go postings — that is the established
meaning of the chip and the reason it is a link at all. The definition is the secondary
read, so it lives in the reveal, with "What is Go? →" pointing at `/skills/go`.

*Consequence:* the glossary pages get few internal links from the hottest page on the
site. Accepted: the sitemap is the crawl path, exactly as it is for `/roles`, and the
`/skills` index links all of them.

### Every described skill gets a page; only the postings block is gated

The route 404s on a slug that is not a described canonical. A described skill with few
open postings still serves its page — definition, the spellings the parser accepts,
neighbouring skills — and simply omits the postings block, mirroring
`roleLandings.MIN_SALARY_SAMPLE`. Floor: 25 open postings, the `/insights` figure
`roleLandings.ts` cites.

*Why not gate the whole page on postings, like `/roles` gates a pair:* a `/roles` pair
page is *about* the postings; a glossary page is about the definition. Gating it would
mean the tooltip's "What is X?" link disappears for exactly the obscure skills a reader
is most likely not to recognise.

*Thin-content risk is real and answered with facts, not filler.* The aliases block comes
free from the alias tables through a new `skilltag.Aliases(canonical)`, since they are
unexported — but **it carries far less than this section first assumed**. Measured after
group 1 landed: 1,038 aliases across 863 canonicals, and **552 of them (64%) have no
spelling beyond the slug and the label**. `Aliases("javascript")` is exactly
`["javascript"]`, so an unconditional block would read "also written as: javascript"
under a heading that says JavaScript — filler on two pages in three.

Two consequences, both settled here rather than discovered in task 7:

- **The aliases block is gated like the postings block**, on having at least one spelling
  that is neither the slug nor the label. On the majority of pages it simply does not
  render.
- **`Aliases("1c")` is `["1c", "1с"]`** — Latin and Cyrillic `с`, which render
  identically. A spelling that differs from another only by an invisible codepoint is
  dropped from the *display* list (never from the parser), or the block shows the same
  word twice and reads as a bug.

What actually carries a sparse page is therefore the definition, the live posting count
with a link to the filter, and neighbouring skills — not the aliases. The aliases are a
bonus where they exist.

## Risks / Trade-offs

- **LLM-written definitions can be confidently wrong** (a niche ERP module, a
  same-named library and product) → the generator prints; a human edits and merges.
  Waves are ordered by frequency, so the skills most readers meet get the most careful
  eyes. A wrong definition is a one-line fix with no deploy coupling.
- **863 sentences is a long programme that can stall half-done** → the ratchet makes a
  half-done state correct rather than broken: undescribed skills render exactly as they
  do today. The endgame task is in this change's task list so the state is not left
  ambiguous.
- **The touch change touches every tooltip on the site** → shared-component tests cover
  the existing paths; the new path is additive and gated on pointer type.
- **A new sitemap shard grows the index** → 863 URLs is one file, well under the
  50,000-URL limit, and one entry in the index. `/roles` already adds ~2,200 across
  shards.
- **The dynamic import means the first hover has latency** → the chunk is a few tens of
  KB and cached for the session; the alternative costs every visitor on every page.

## Migration Plan

No data migration. Deploy order does not matter: an SPA build without the descriptions
module renders chips exactly as today, and the Go dictionary is inert until something
reads it.

Rollback is a revert. The routes and the reveal are additive; nothing existing changes
behaviour except the job chip's label, which is a bug fix in the same direction as the
rest of the product.

## Open Questions

None blocking. Two settled above that a reviewer might reopen: the postings floor (25)
and the wave sizes (100 / 200 / the tail) are both single constants, cheap to change
after the first wave shows how the text reads.
