## Why

Creative-media work is invisible in this catalogue. A "Video Editor",
"Videographer", "Animator", "3D Artist", "Concept Artist" or "Photographer"
resolves to NO category and NO named role — the title dictionary carries none of
these words, so roughly 830 open postings sit unfilterable, exactly the silent
hole the design split found last time ("BIM Modeler" resolved to nothing, and
widening the dictionary returned more than the split itself).

Two nearby titles are worse than missing: `Sound Designer` and `Audio Designer`
currently resolve to `design`, so audio work is filed beside UX work purely
because the word "designer" is in the title.

## What Changes

- Add a `creative` category to the facet vocabulary for media production —
  video, animation, art, audio and photography — and place it in the
  **technical** category set, so its postings are enriched and embedded like
  `design` and `product`.
- Add title aliases resolving to `creative`: the video family (editor,
  producer, videographer), the art family (2D/3D, concept, character,
  environment, technical, VFX, storyboard), animators, the audio family, and
  photographers.
- Move `sound designer` / `audio designer` out of `design` into `creative`.
- Add named roles for the new crafts, and — without a new category — for the
  game-development titles that today collapse into a bare `design` or
  `software_engineering`: game designer, level designer, narrative designer,
  game producer, game developer.
- Add the creative toolchain to the skill dictionary (DaVinci Resolve, Final
  Cut Pro, Cinema 4D, CapCut, Godot, Houdini, Nuke, Substance Painter, ZBrush,
  colour grading, storyboarding, video editing), gating the ambiguous ones.
- Label the new category and give it its own group in the web category picker.

Not breaking. `motion designer`, `graphic designer`, `visual designer` and
`brand designer` deliberately stay in `design` — this change adds a category
for work that resolves to nothing today rather than re-cutting a facet users
already filter and link to.

## Capabilities

### New Capabilities
- `creative-media-taxonomy`: which titles resolve to the `creative` category,
  why it is technical, the named roles the media crafts expose (including the
  game-development titles that keep their existing categories), and the curated
  creative-toolchain skill vocabulary.

### Modified Capabilities
- `design-taxonomy`: "Sound Design Engineer" is currently pinned to `design`.
  It moves to `creative` with the rest of audio, and "Audio Design Engineer"
  moves with it — otherwise one craft scatters across three categories, the
  second spelling falling through the bare "design engineer" alias into
  draughting.

<!-- `tech-classification`, `role-facet`, `skill-tag-matching` and
     `facet-display-labels` state mechanisms that already admit a new value;
     none of their requirements change. -->

## Impact

- `internal/dict/vocab` — `CategoryValues`, `TechCategories`. No migration and
  no `CHECK` constraint exists on `jobs.category`; the vocabulary is enforced in
  Go only.
- `internal/dict/classify` — the title `categoryTable`, ordered so a longer
  qualified alias never loses to a shorter rival (`illustrator` must not steal
  "Graphic Designer, Illustrator").
- `internal/dict/roletag` — new named-role entries.
- `internal/dict/skilltag` — new canonicals plus `ambiguousWords` gating.
- `cmd/gen-contracts` output (`web/src/lib/generated/contracts.ts`), plus
  `web/src/lib/labels.ts` and `web/src/lib/filterSections.ts` — a category
  absent from the section map is unselectable, a defect only `svelte-check`
  caught last time.
- Rollout: `backfill-derive` re-derives `category`/`skills`/`is_tech`
  (~3.5 h over 5.4M rows), then a plain `make reindex` with the reindex timer
  stopped. Roles need only the reindex — they are derived at index time.
- Cost: the newly-`creative` postings become `is_tech = true` and therefore
  enter the enrichment queue. At ~830 open postings this is a one-off spend,
  not a standing one.
