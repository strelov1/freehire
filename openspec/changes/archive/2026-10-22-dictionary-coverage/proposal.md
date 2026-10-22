## Why

Facet dictionaries drop anything they do not already know. They grow only when a maintainer edits them by hand or an agent mines what a model already emitted. People who notice a missing specialization or skill have nowhere to say so, and no page shows what the dictionaries hold.

## What Changes

- Add a public `/dictionary` page with two sections, Specialization and Skills. They use the same components as the job filter modal: `CategoryPane` for specializations and `SearchSelect` with tech icons for skills.
- List what the dictionaries hold, not only what has open jobs. Open-job counts come from one unfiltered facet call and are best-effort.
- Picking chips shows a "Show jobs" link to `/jobs` with those filters.
- When a search matches nothing, show "“term” is not in the dictionary." and an "Add to the list" button. A request is recorded only on click, into a review queue. Nothing is written into a dictionary.
- Add `POST /api/v1/dictionary/gaps`. The server checks the term against the real dictionary and refuses one it already resolves.
- Store one live row per `(kind, normalized term)`, and increment a counter when the same term is asked again. A rejected term can be asked again; an accepted one keeps counting.
- Recording requires a signed-in user, capped at 20 writes per UTC day, with no credit reward.
- Link the page from the footer Product group, after Skills glossary.

## Capabilities

### New Capabilities

- `dictionary-coverage`: the public page listing recognised specializations and skills, and the queue a missing term lands in.

### Modified Capabilities

## Impact

- New tables `dictionary_gaps` and `dictionary_gap_daily`, one migration (`0188`).
- `POST /api/v1/dictionary/gaps` (cookie auth). No read route for the queue.
- `FoldTerm` and `GapTerm` beside `internal/dict/classify`, taking no database. The handler calls them, then sqlc.
- `SearchSelect.svelte` and `CategoryPane.svelte` gain a bindable search text and an optional `empty` snippet.
- Page at `web/src/routes/dictionary/` and one footer link.
- Maintainers read and update the queue with SQL. No moderation screen.
