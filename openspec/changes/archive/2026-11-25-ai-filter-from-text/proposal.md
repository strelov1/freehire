## Why

The job filter carries ~25 facets. Someone who knows exactly what they want — "senior Go
backend, remote, somewhere in Europe, not a bank, posted this month" — has to translate
that sentence into six separate facet selections before the list means anything. Most
never finish the translation: they browse an unfiltered catalogue and conclude we carry
nothing for them.

Let them write the sentence and fill the filters for them.

## What Changes

- A **"Describe with AI"** entry point in the job filter sidebar, above the existing
  All-filters button. Visible to everyone; a signed-out click opens the existing auth
  dialog rather than a bespoke gate.
- A dialog with two ways to seed a search: **free text**, or **the caller's saved
  profile + structured CV**.
- One model call turns that into **canonical facet values**, a summary sentence, and a
  list of what it could not resolve. Every open-vocabulary value (skills, countries,
  cities) is canonicalised server-side through the dictionaries that already own those
  vocabularies; anything a resolver refuses is dropped AND reported, never guessed. A
  company name is the one filter this surface cannot ground — no dictionary here tells
  "Stripe" from "Stripee" — so it is always dropped and reported rather than resolved.
- A **preview** step: the summary sentence, the resolved values as chips, the
  "didn't recognise" line, and one field to refine ("also remote only") which re-runs the
  interpretation with the previous result as context.
- **Apply** replaces the whole filter state, then hands off to the existing sidebar: the
  values become ordinary removable chips, and the `SaveSearchAlert` control already
  sitting under the same button saves the result as a search alert. No new persistence.
- New endpoint `POST /api/v1/search/interpret`, cookie-authenticated and rate-limited per
  user. Spend goes on the caller's own gateway credential under a new `feature:` tag, as
  the LLM attribution convention requires.

Not in scope: a multi-turn chat with tool calls. The in-app assistant already does that at
a different cost and latency. This is one call that returns a filter, plus an optional
second call to refine it, with no transcript and nothing stored.

## Capabilities

### New Capabilities
- `ai-filter-intent`: turning a natural-language search description (or a saved profile)
  into canonical, filter-ready facet values — the grounding rule that no value reaches the
  filter unresolved, the unresolved report, the summary sentence, and the refine round.
- `ai-filter-entry`: the sidebar entry point and its dialog — visibility to everyone,
  the signed-out auth handoff, the two seed tabs, the preview/refine/apply states, and
  how an applied result lands in the filter store.

### Modified Capabilities
<!-- None. The filter sidebar gains an entry point but no existing requirement changes:
     applying writes through the filter store's published operations, and saving is the
     unchanged save-search-alert capability. -->

## Impact

- **New Go package** `internal/searchintent` — prompt, structured-output schema, and the
  resolution pass. Depends on `internal/vocab`, `internal/skilltag`, `internal/location`,
  `internal/industrytag`, `internal/normalize`, `internal/llm`. Imports no handler, so it
  is testable against a fake model with no HTTP.
- **`internal/handler`** — a new transport-only handler + limiter, registered beside the
  search routes; one new feature tag constant in `user_llm.go`.
- **`web/`** — `FilterSummaryShell.svelte` gains a `beforeButton` snippet;
  `FilterSummary.svelte` fills it; new `AiFilterButton.svelte` and `AiFilterDialog.svelte`;
  one new API client call.
- **No migrations, no new tables, no queue.** Nothing about the result is persisted.
- **openapi.yaml** — the endpoint is part of the published integration contract and is
  documented there.
