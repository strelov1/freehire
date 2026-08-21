# AI filter — describe a search in words, get facets

Date: 2026-08-21

## Problem

The job filter has ~25 facets. A person who knows what they want ("senior Go
backend, remote, somewhere in Europe, not a bank, posted this month") has to
translate that into six separate facet selections before the list means
anything. Most never finish the translation, so they browse an unfiltered
catalogue and conclude we have nothing for them.

Give them a text box: they describe the search, we fill the filters.

## Scope

In:

- A "Describe with AI" entry point in the job filter sidebar, visible to
  everyone, usable only when signed in.
- Two ways to seed it: free text, or the caller's saved profile + CV.
- A preview of what we understood, one text field to refine it, then apply.
- Applying replaces the whole filter state.

Out:

- Multi-turn chat with tool calls. The in-app assistant
  (`internal/assistant`) already does that, at a different cost and latency.
  This is one model call that returns a filter, plus an optional second call
  to refine it.
- Any new persistence. Saving the result is the existing `SaveSearchAlert`
  control, which already sits in this sidebar.

## The grounding rule

This is the part that decides whether the feature is useful or a confident
liar.

`search.FilterFromValues` ignores parameters and values it does not
recognise. A model that emits `skills=python3` or `countries=Portugal`
(rather than `pt`) therefore produces *an unfiltered result set that looks
like an answer* — the exact failure the `UnknownParams` echo was built to
expose on the public search. So no value the model writes reaches the filter
unresolved.

Two vocabularies, handled differently:

**Closed facets** — small, enumerable, listed inline in the prompt:
`work_mode`, `seniority`, `category`, `employment_type`, `role_type`,
`english_level`, `education_level`, `relocation`, `regions`,
`company_type`, `company_size`, `salary_currency`, `salary_period`. All read
from `internal/vocab`, which is already the single definition these values
have. A value outside the list is dropped.

**Open facets** — too large to enumerate, so the model writes ordinary words
and the server canonicalises with the dictionaries that already exist:

| Facet | Resolver |
|---|---|
| `skills` | `skilltag.Canonicalize` (alias → canonical) |
| `countries`, `cities` | `location.Parse` / `location.SearchCities` |
| `domains` | `industrytag.Canonicalize` |
| `company_slug` | `normalize.CompanySlug` + `company_slug_aliases` |
| `collections` | exact match against the curated tag list |

Anything a resolver refuses is **dropped and reported**, never guessed. The
response carries an `unresolved` list, and the dialog shows it: "didn't
recognise: *blockchain-adjacent*". A silent drop here is the same lie as a
hallucinated facet.

Scalars (`salary_min`, `posted_within_days`, `experience_years_max`,
`visa_sponsorship`) are bounded numerically rather than by dictionary.

Free text `q` is allowed for one purpose only: a concept no facet covers. The
prompt says so, and the preview shows it as its own chip so it can be
removed. Everything a facet can express must go through the facet — a `q`
that duplicates a facet narrows the results twice.

## Backend

New package `internal/searchintent`. It owns the prompt, the structured
output schema, and the resolution described above. It depends on `vocab`,
`skilltag`, `location`, `industrytag`, `normalize` and an `llm.Client`; it
does not import `handler`, so it is testable with a fake model and no HTTP.

```go
type Request struct {
    Text     string    // what the user typed; empty when seeding from profile
    Profile  *Profile  // specializations, skills, excluded skills, location prefs, CV headline
    Previous *Result   // the refine round: what we last understood
}

type Result struct {
    Facets     map[string][]string // resolved, canonical, filter-ready
    Query      string              // free text, only for what no facet covers
    Scalars    Scalars             // salary_min, posted_within_days, experience_years_max, visa
    Summary    string              // one human sentence: what this search is
    Unresolved []string            // what we dropped, verbatim as the model wrote it
}
```

`Summary` is what the preview shows as prose. It is generated in the same
call as the filter, so it can never describe a different search than the one
we resolved — a second call to summarise could drift.

**Endpoint:** `POST /api/v1/search/interpret`, `RequireAuth` (cookie only —
this is a browser surface, not an integration one), rate-limited per user
in the shape of `matchAnalysisLimiter`. Body: `{"text": "...", "source":
"text" | "profile", "previous": {...}}`. Text capped at 1000 characters.

The handler is transport only: read the caller, build the `Request`, call the
service on a client bound with `userLLM(ctx, keys, client, userID,
tagSearchIntent)`. `tagSearchIntent = "feature:search-intent"` joins the
constants in `internal/handler/user_llm.go` — every model call made for a
signed-in user spends on that user's own gateway credential, and this one is
no exception.

Search unconfigured or LLM unconfigured: 503, the same as the other
search-dependent routes.

## The profile tab

"From Profile" builds the same `Result` from what the user has already told
us — `userprofile.Get` (specializations, skills, excluded skills, location
preferences) plus the structured CV headline and years, if present. This is
the same material `get_profile` reads for the assistant, so the two surfaces
cannot disagree about what a profile says.

The profile's own fields are largely canonical already (specializations are
category values, skills are canonical tags), so most of it maps without a
model call. The model is still called: it turns "5 years, mostly Go and
Kubernetes at fintechs" into a seniority, a category and a domain, and it
writes the summary sentence.

A caller with no saved profile gets a 404 with a message pointing at
`/my/profile`, and the tab renders that as an empty state rather than an
error — the same treatment `noProfileResult` gives the assistant.

## Frontend

**Entry point.** `FilterSummaryShell.svelte` gains a `beforeButton` snippet
next to the existing `afterButton`, and `FilterSummary.svelte` fills it with
`AiFilterButton.svelte`. Visible to everyone. The company filter summary
passes nothing, so it is unaffected.

**Not signed in.** The click opens the existing `AuthDialog` via
`auth-dialog.svelte.ts`. No bespoke gate, no second sign-in surface.

**Signed in.** `AiFilterDialog.svelte`, two tabs (Text query / From profile),
in three states:

1. *Input* — textarea, three example queries, "Build filter".
2. *Preview* — the summary sentence, the resolved values as chips grouped
   like the sidebar, the "didn't recognise" line if any, a "add a
   constraint…" field, and two buttons: "Apply" and "Refine".
3. *Error* — the request failed, or nothing resolved at all. "Nothing
   resolved" is its own message ("I couldn't turn that into filters — try
   naming a role, a skill or a place"), not a generic failure.

Refining posts the same endpoint with `previous` set, and replaces the
preview. There is no transcript and nothing is stored; closing the dialog
discards it.

**Applying.** `store.clear()`, then the resolved values through the store's
existing `add` / `setSalaryMin` / `setPostedWithinDays` /
`setExperienceYearsMax` / `setVisa` / `setQuery`. The URL and the result list
follow from the store as they already do — this feature writes no new
plumbing to make results appear.

Once applied, the chips are ordinary filter chips: removable one by one in
the sidebar. And `SaveSearchAlert`, already sitting under the same button,
saves the result as a search alert. That is the whole "save it after" story;
no new code.

## Testing

- `internal/searchintent`, unit, fake model: a hallucinated facet name is
  refused; an unresolvable skill is dropped and reported; a country written
  as a name resolves to its code; a value outside a closed vocabulary is
  dropped; `q` survives only when no facet covers it; scalars out of range
  are dropped.
- Handler, integration-tagged: unauthenticated is 401; over the rate limit is
  429; profile source with no profile is 404; LLM unconfigured is 503.
- Web, vitest: the preview → store application writes exactly the resolved
  values and nothing else; a preview with only unresolved values offers no
  Apply.

## Risks

- **A filter that resolves to nothing.** The model understood, the catalogue
  has no such job. The preview cannot know — it does not run the search. The
  sidebar's existing zero-result state carries it, and the filters are right
  there to loosen.
- **Cost.** One call per build and per refine, on the user's own credential
  and tagged, so the spend is attributable. The rate limiter bounds a loop.
- **Prompt injection.** The input is 1000 characters of user text going to a
  model whose output only ever reaches a dictionary resolver. There is no
  tool call, no fetch, and no privileged action behind it — the blast radius
  of a successful injection is a filter the user can see and remove.
