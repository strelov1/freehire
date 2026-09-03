## Context

`web/src/lib/components/JobApplyForm.svelte` renders the served form as a caption,
one line of standard fields, and a flat `<ul>` of questions. The served shape
(`internal/ingest/applyform.Display`) already carries everything the new rendering
needs — each question's text, whether it is required, and a name for the kind of
answer it expects — drawn from a closed vocabulary in `display.go`:

| `answer` value    | control kind      |
| ----------------- | ----------------- |
| `""` (absent)     | one-line answer   |
| `"choose one"`    | select            |
| `"choose any"`    | multi-select      |
| `"yes / no"`      | boolean           |
| `"written answer"`| textarea          |
| `"upload"`        | file              |

The absent value is deliberate on the Go side: a one-line answer is what anyone
assumes, so naming it is noise, and a control kind the capture could not normalize
is given no word rather than the nearest one.

Two pieces this change needs already exist and are not being built: `BrandMark`
(`design-system/src/brand-mark.svelte`), which draws a brand glyph from a path and
a hex; and `simple-icons`, already a dependency of `web/` and already used this way
by `web/src/lib/techmarks.ts` through `SkillIcon.svelte`.

## Goals / Non-Goals

**Goals:**

- The reader learns the form's cost — how many questions, how many essays — before
  reading any question.
- The kind of answer is stated once per group instead of once per question, which
  removes the trailing hint that currently wraps onto its own line.
- The provider caption carries a recognisable mark where one exists.

**Non-Goals:**

- Changing `internal/ingest/applyform`, the `/api` wire shape, or `openapi.yaml`.
- An estimate of minutes. The counts are measured; a duration would be invented.
- Marks for every ATS. Coverage is whatever `simple-icons` verifiably carries.
- Any change to `applyFormWorthShowing` or to which postings show the block at all.

## Decisions

### Group in a pure module, not in `ForDisplay` and not in the component

Grouping is a pure function in a new `web/src/lib/applyFormGroups.ts`, over the
questions as served; the component calls it from a `$derived` and does nothing but
lay the result out.

The seam is not stylistic. `web/vitest.config.ts` runs the web suite in plain Node
with no Svelte compilation — its own comment says as much, and names `facetModel.ts`
as the precedent: the logic worth testing lives in a module with no runes and no
`$app/*` imports. Logic written inside a `$derived` in a `.svelte` file is
unreachable by any test in this repo. Since the counts, the group order and the
single-group collapse are exactly the parts with edge cases worth pinning, they
have to live where a test can reach them.

The alternative — partitioning in `ForDisplay` and serving groups — was rejected
because `Display.Questions` is documented as "the employer's own, in the order the
form presents them", and `web/static/openapi.yaml` is the integration contract
other consumers read. Reordering the array to suit one reader's layout would make
that documented promise false for everybody, to save a `reduce` in one component.
The Go projection stays the honest record of what the employer published; the
page decides how to read it.

This also keeps the decision reversible. If grouping turns out to be the wrong
call, it is one component's `$derived` block, not a wire change and a re-capture.

### Four groups, keyed by the closed `answer` vocabulary

`Short answers` (`""`), `Pick from a list` (`choose one` / `choose any` /
`yes / no`), `Written answers` (`written answer`), `Attachments` (`upload`), in
that order — the order is the point, since it runs cheapest to most expensive.

The mapping is a total function over a closed vocabulary, with the empty string as
its own case rather than a fallback. Any value the map does not know falls into
`Short answers`, which is the same assumption `display.go` makes when it declines
to name a kind: an unqualified question is assumed answerable in a line.

Three groups rather than four was considered — folding `Attachments` into
`Written answers` as "the expensive ones". Rejected: an upload is a file the
candidate either has or does not, which is a different kind of cost from writing
prose, and `upload` questions are rare enough that the group is usually absent
anyway.

### Headings collapse when there is only one group

If exactly one group is non-empty, no headings render. A single heading reading
`Written answers (5)` directly beneath a summary reading `5 questions · 5 written
answers` says the same thing twice in two lines.

### The mark accompanies the name; the name never leaves

`simple-icons` carries `siGreenhouse` and nothing for Ashby, Workable, Lever or
Recruitee. `siGreenhouse` was verified to be the ATS rather than a slug collision:
its `source` is `brand.greenhouse.io/brand-portal`. `techmarks.ts` records three
cases where an exact slug match resolved to the wrong brand (`elk`, `hive`,
`backbone`), so the source check is the established bar here, not extra caution.

With one mark in five, replacing the provider's name with its mark would leave
four postings in five with an unattributed block. So the mark is additive, and its
absence renders nothing at all — no placeholder glyph, matching how `SkillIcon`
handles a skill with no mark.

The map lives in a new `web/src/lib/atsmarks.ts` rather than inline in the
component or appended to `techmarks.ts`. Inline hides the coverage reasoning
inside a component that is about layout; `techmarks.ts` is explicitly scoped to the
skills dictionary and mixing ATS providers into it would make its lookups
ambiguous. A separate small named map is the shape this repo already uses for
exactly this (`techmarks.ts`, `familymarks.ts`, `backers.ts`).

### The index key stays an index key

`{#each form.questions as question, i (i)}` is keyed by position on purpose:
Greenhouse and Workable both publish the same screening question twice on some
postings, and a duplicate key throws `each_key_duplicate` during Svelte 5
hydration rather than warning — which took the whole job page down once.

Grouping partitions the list into new arrays, so the keying question is asked
again per group. Each group's `{#each}` is keyed by its own index, which stays
sound for the same reason the original is: the lists are inert, rebuilt wholesale
whenever `form` changes, never reordered or filtered in place.

## Risks / Trade-offs

- **The employer's ordering is lost between groups.** → Accepted, and stated in
  the proposal. Preserved *within* each group, and the form is actually filled on
  the platform's own site where the platform's order governs.

- **A form of two questions gets three headings' worth of chrome.** → The
  single-group collapse covers the degenerate case. A two-question form spanning
  two groups renders two headings, which is proportionate.

- **`simple-icons` removes marks on trademark request** — it has already dropped
  AWS, Java and C#, as `techmarks.ts` records. A future version could drop
  Greenhouse. → The absence path is the same one four of five providers already
  take, so the block degrades to text and nothing breaks. No guard needed beyond
  the lookup returning `undefined`.

- **The summary and the list could disagree** if the counts were computed from
  anything but the rendered questions. → Both are `$derived` from the same
  `form.questions`, so there is no second source to drift from.

## Migration Plan

None. Front-end only, no schema, no stored data, no worker. Rollback is reverting
the commit.
