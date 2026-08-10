## Context

Job-filter query params are list-valued: a facet like `skills` can carry dozens
of selected values. Today both the web app (`web/src/lib/facetModel.ts`) and
the backend (`internal/search/query_filter.go`) only understand the
repeated-key encoding, `skills=go&skills=react&skills=aws`. Every consumer of
that URL shape — the web app's own filter bar, saved `search_profiles`,
`notify` subscription alert URLs (`internal/notify/match.go` replays the
stored query string through the same `FilterFromValues`), and third-party
clients such as `../freehire-cli` — must keep working unmodified.

The backend reads query params through `queryValues(c)`
(`internal/handler/helpers.go`), which uses `net/url.ParseQuery` specifically
to preserve repeated keys (Fiber's own accessors collapse them). That parsed
`url.Values` flows into `search.FilterFromValues`
(`internal/search/query_filter.go`), which resolves each facet in the
`StringFacets` map by reading `v[param]` as a slice.

## Goals / Non-Goals

**Goals:**
- Accept a compact comma-separated form (`skills=go,react,aws`) for every
  list-valued facet param (every entry in `StringFacets`, both the include
  param and its `_exclude` counterpart).
- Keep accepting the existing repeated-key form with no behavior change for
  any existing caller.
- Make the web app's own filter bar emit the compact form going forward.
- Do this with one parsing path, not two — so there is no "legacy branch" to
  track or remove later.

**Non-Goals:**
- Company filters (`stagedCompanyFilters.ts` / `companyFilters.ts`) — a
  separate codec with its own param set; out of scope for this change.
- Changing `../freehire-cli` — it already works unchanged against a
  backward-compatible backend; revisit only if it's later found to break.
- A version flag, feature flag, or content-negotiation mechanism to pick a
  format. Both formats are always accepted.
- Comma-escaping within a single facet value. All current `StringFacets`
  values are dict-only kebab-case slugs (see `internal/search/query_filter.go`
  and the frontend facet dictionaries) — none legitimately contain a literal
  comma, so no escaping scheme is needed.

## Decisions

**One flatten step, not a format switch.** Instead of detecting "is this
comma-separated or repeated-key" and branching, both the backend and the
frontend apply the same transform to whatever raw values they read for a
param: split every individual value on `,`, flatten the results into one
list, drop empties from stray commas. A `url.Values`/`URLSearchParams` for
`skills=go&skills=react` yields raw values `["go", "react"]`, each with no
comma — splitting is a no-op and flattening returns them unchanged. A
`skills=go,react` yields one raw value `["go,react"]`, split into `["go",
"react"]`. Mixed input (`skills=go,react&skills=aws`) resolves correctly too.
The alternative — detect-and-branch — would work equally well today but
leaves a format check as permanent code and as something to eventually decide
to prune; unifying the two forms into one code path means there is nothing
to remove later, addressing that concern from the outset.

- Backend: `internal/search/query_filter.go`, `filterFromValues` — replace the
  direct `v[param]` (and `v[param+"_exclude"]`) reads with a small
  `splitFacetValues(raw []string) []string` helper that does the split-and-
  flatten, applied identically to include and exclude params for every facet
  in `StringFacets`.
- Frontend: `web/src/lib/facetModel.ts` —
  - `filtersToParams` (serialize): for each facet with selected values, write
    one `p.set(def.param, values.join(','))` instead of repeated
    `p.append(def.param, v)`; same for `${def.param}_exclude`. Omit the param
    entirely when there are no selected values (existing behavior).
  - `filtersFromParams` (parse): replace `p.getAll(def.param)` with
    `p.getAll(def.param).flatMap(v => v.split(',')).filter(Boolean)` — the
    same split-and-flatten shape as the backend, kept as one small shared
    helper rather than duplicated inline in both functions.
- Docs: `web/src/lib/docs/filters.ts` — the worked example changes from
  `skills=go&skills=rust` to `skills=go,rust`, with a line noting the
  repeated-key form is still accepted. `docs/API.md` picks this up through the
  existing generator (`gen:api-docs`), no separate hand-edit.

**Comma as the separator.** Matches the user's own mental model
(`skill=[go,react]`) and is the conventional choice for compact multi-value
query params. Confirmed safe because every `StringFacets` value is a
dict-only slug (no free text, no commas).

**Scope: all `StringFacets`, not just `skills`.** `filterFromValues` already
treats every facet in that map uniformly; special-casing only `skills` would
mean a second code path for everything else and no real reduction in
complexity. Applying the same helper to all of them is not more work than
scoping it down.

## Risks / Trade-offs

- **A dict-value later gains a legitimate comma** → would silently split into
  two facet values. Mitigation: `StringFacets` values are dict-only and
  validated at the source (skill/region/category dictionaries) — a comma
  would already be an odd dictionary entry; no such value exists today and
  none of the dictionary-generation code introduces one.
- **A client sends both a comma-joined value and a repeated key for the same
  param** (`skills=go,react&skills=aws`) → resolves as the union of both
  (`go`, `react`, `aws`), matching the OR semantics the facet already has
  for repeated keys. This is a superset behavior, not a break.
- **`freehire-cli` or another external client parses the URL it built itself
  and assumes repeated-key shape** → not affected, since nothing forces a
  caller to send the compact form; the backend still accepts (and the web
  app still round-trips) whatever a caller already sends.

## Migration Plan

No data migration. This is additive on the parsing side (a strict superset of
accepted input) and a serialization-only change on the web app's filter bar.
Deploy order: land the backend change first (or together — the change is
backward compatible either order, but landing backend-first means the
frontend's new compact links are understood immediately when it ships).
Rollback is a plain revert; no stored data depends on the new encoding since
saved `search_profiles`/subscription query strings are stored verbatim and
replayed through the same now-unified parser regardless of which form they
were captured in.

## Open Questions

None — resolved during brainstorming with the user:
- Backward compatibility: support both formats indefinitely (not a
  time-boxed deprecation).
- Scope: all `StringFacets`, not just `skills`.
- Nothing to prune later on the code side, per the "Decisions" section above;
  the only future follow-up is dropping the repeated-key mention from docs
  once external consumers are confirmed migrated, which is a documentation
  edit, not a code change.
