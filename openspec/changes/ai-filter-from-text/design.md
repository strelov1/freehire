## Context

The job filter carries ~25 facets across `search.StringFacets` plus the scalar filters.
Assembling a real search takes six or more selections spread over a modal and a sidebar.

Three pieces of the machinery this needs already exist:

- **A filter vocabulary the model can be held to.** `internal/handler/assistant_tools.go`
  defines `assistantFilterArgs`, whose `values()` refuses any facet outside
  `search.StringFacets` precisely because `FilterFromValues` ignores what it does not
  recognise, and a hallucinated filter would come back as an unfiltered result set that
  looks like a confident answer. The same reasoning drives this change; the difference is
  that this surface must also canonicalise *values*, not just names.
- **Dictionaries that own each open vocabulary.** `skilltag.Canonicalize`,
  `location.Parse` / `location.SearchCities`, `industrytag.Canonicalize`, and the company
  slug rule with `company_slug_aliases`. All of them already resolve alias → canonical and
  drop what they cannot place.
- **A sidebar with the right shape.** `FilterSummaryShell.svelte` renders the chips and an
  `afterButton` snippet that already holds `SaveSearchAlert`. Applying a filter and saving
  it are solved problems; this change only has to produce the values.

Design record for the shaping conversation:
`docs/superpowers/specs/2026-08-21-ai-filter-design.md`.

## Goals / Non-Goals

**Goals:**

- One sentence in, a correct filter out — with an honest report of what was not understood.
- No value reaches the filter unresolved.
- The result is an ordinary filter afterwards: removable chips, a shareable URL, savable
  through the existing alert control.
- Per-user model spend, attributed and tagged like every other per-user model call.

**Non-Goals:**

- A conversation. The in-app assistant (`internal/assistant`) is the multi-turn,
  tool-calling surface. This is one call plus an optional refine, with no transcript and
  nothing stored.
- Running the search. The interpretation returns filters; the existing list renders them.
- Any new table, queue, or migration.

## Decisions

### The model proposes; the dictionaries dispose

The model never writes a facet value directly into a filter. Its output passes through a
resolution stage that canonicalises open-vocabulary values with the existing dictionaries
and validates closed-vocabulary values against `internal/vocab`. Anything unresolved is
dropped **and reported**.

*Alternative considered:* enumerate every vocabulary in the prompt, including skills, and
trust the model to pick only listed values. Rejected — the skill dictionary alone runs to
thousands of canonicals, the token cost lands on every request, and a model that copies a
listed value 99% of the time still produces the silent-unfiltered failure the other 1%.
Resolution after the fact is cheaper and cannot fail open.

*Alternative considered:* let unresolved values through as free-text `q` terms. Rejected —
a `q` term is an AND against the whole document, so an unrecognised term silently empties
the result set. Reporting the drop tells the truth; smuggling it into `q` does not.

### One call returns the filter and its summary

The summary sentence the preview shows is a field of the same structured response as the
facets. A second call to describe the result could describe a different search than the
one resolved, and the caller would have no way to tell.

### `internal/searchintent`, not more of `internal/handler`

Prompt, schema, and resolution live in a new package that imports the dictionaries and
`internal/llm` and nothing from the HTTP layer, so the whole behaviour is testable against
a fake model with no server. The handler reads the caller, builds the request, binds the
client with `userLLM(...)`, and returns — transport only.

The interpretation shape is deliberately *not* `assistantFilterArgs`. That type is the
assistant's tool argument and carries the assistant's constraints; sharing it would couple
two surfaces that need to evolve separately. What they do share is `search.StringFacets`,
the actual vocabulary, which both consult.

### Cookie-only authentication

`RequireAuth`, not `RequireAuthOrKey`. This is a browser surface; an integration that
wants filters has the documented facet vocabulary and `/agent/jobs/search`. Cookie-only
keeps the model spend attached to an interactive session, where the rate limiter's
per-user shape means something. Widening it later is additive.

### Refinement is stateless

A refine posts the previous result back and receives a complete replacement. No session,
no transcript, no row. The caller always sees one coherent search rather than a diff, and
closing the dialog costs nothing to clean up.

### Applying replaces rather than merges

`store.clear()` then write. A merge would intersect the user's stale facets with the new
ones and produce empty result sets that look like "we have nothing" — the failure this
feature exists to fix.

### The entry point renders for signed-out visitors

Visible to everyone, gated on activation through the existing auth dialog. A feature
hidden from signed-out visitors is hidden from exactly the people it could convert.

## Risks / Trade-offs

- **A perfectly-understood filter that matches nothing.** The preview does not run the
  search, so it cannot warn. → The sidebar's existing zero-result state carries it, and
  every value is one click from being removed. Running a count in the preview would double
  the latency of the common case to serve the rare one.
- **Model spend per click, twice with a refine.** → Bound by a per-user limiter in the
  shape of `matchAnalysisLimiter`, and spent on the caller's own gateway credential under
  `feature:search-intent`, so the cost is attributable rather than anonymous.
- **Prompt injection through 1000 characters of free text.** → The model's output only
  ever reaches a dictionary resolver. No tool call, no fetch, no privileged action. The
  blast radius of a successful injection is a filter the user can see and remove.
- **Resolution silently narrowing what the model meant.** A dropped value the caller does
  not notice is the same lie as a hallucinated one. → The unresolved list is part of the
  response contract, and the preview renders it; a spec scenario pins both.
- **Profile-seeded results going stale.** The profile is read at interpretation time, so a
  saved alert built from it does not track later profile edits. → Acceptable: a saved
  search is a snapshot everywhere else in the product too.

## Migration Plan

No schema change, no backfill, no worker. Ship the endpoint before the UI (the UI is the
only caller), configure nothing new — the LLM client and gateway resolver the handler
needs are already wired for the assistant and match analysis. Rolling back is removing the
route and the sidebar snippet.

`openapi.yaml` is the published integration contract and gains the endpoint in the same
change.

## Open Questions

None blocking. Two settled during shaping and recorded here so they are not relitigated:
the profile tab still makes a model call (the profile is nearly canonical, but the
seniority/category inference and the summary sentence are not derivable without one), and
the endpoint is cookie-only.
