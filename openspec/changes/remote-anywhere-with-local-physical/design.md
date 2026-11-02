## Context

`filterFromValues` (`internal/search/search/query_filter.go`) is the single pure
builder the HTTP handler, the saved-search digest matcher and `Narrows` share. Its
grammar: within a facet, included values OR into one group; facets AND across
groups; the location facets (regions/countries/cities/metros) share ONE ORed group.
The wanted predicate — remote anywhere, OR physical work within the location
facets — is a disjunction whose right side ANDs two existing groups, which that
grammar cannot state. This change widens the language with one modifier rather than
a general nested-filter syntax: one new boolean param, read by exactly the function
that already reads every other.

Meilisearch accepts arbitrary parenthesized boolean expressions inside a filter,
including inside one OR-group — `withOpenJobs` already appends
`NOT is_closed = true AND NOT is_duplicate = true` as a single AND-group
(`closure.go`), so a compound fragment inside `[][]string` is established practice.

## Goals / Non-Goals

**Goals:**
- Express "remote anywhere + hybrid/onsite within the chosen geography" in one
  query, on every surface that parses the canonical query string (search, saved
  searches, digest alerts).
- Keep the modifier inspectable in the filter UI, not a hidden profile side effect.

**Non-Goals:**
- Per-mode geographies beyond the two-way split (e.g. separate city lists for
  hybrid vs on-site). The profile's `base` is one place; the split it motivates is
  exactly remote-vs-physical.
- Changing `work_mode` itself, or the index. The modifier composes existing
  filterable attributes at query time.
- General boolean query composition. One named disjunction, not a query language.

## Decisions

### The modifier rides in `work_mode_remote_anywhere`, a scalarModifier

Not a facet value (`work_mode=remote_anywhere`) — it does not select a mode, it
changes how the facet composes with geography, and a value would have to be
special-cased in every reader of the facet (chips, counts, searchintent's enum).
Not a per-facet `_mode=or` extension — the modifier's meaning involves the
location facets, so it is a property of the QUERY, not of one facet's list.

Like `seniority_include_unspecified`, it is listed in `scalarModifiers`: known to
`UnknownParams`, and alone it narrows nothing (so `Narrows` stays honest without
changes — the same test that guards scalarFilters guards the distinction).

### The disjunction replaces the work-mode group, once, at the end

When the modifier is on AND `remote` is among the included work modes AND there is
at least one location include, the builder emits, in place of the plain work-mode
group:

    work_mode = remote OR (work_mode IN [selected physical modes] AND <location group>)

Only `remote` stands bare: a bare physical mode would admit a hybrid posting from
any country. The right side carries only the physical modes the caller selected,
and the location group is consumed into it rather than ANDed beside it. Location excludes are NOT folded in: they stay their
own AND group, so `countries_exclude=us` keeps excluding the US from the remote
arm too (excluding a country from "remote anywhere" is the normal reading; a
caller who wants remote-US-only can say `countries=us`, which lands on the right
side's geography for the physical arm and its own AND group for remote).

`work_mode_mode=and` disables the modifier: ANDing every selected mode is
incompatible with the disjunction's OR, and the one caller who asks for `and`
(none in the UI) is asking for a different filter. The modifier also does nothing
when `remote` is not among the included modes (the physical arm would relax the
filter the caller wrote) or when no location facet has an include (the right side
degenerates to the physical modes, widening past the caller's list for no reason).

Every other facet keeps its ordinary AND. Two work modes + three countries thus
yield: `wm = r OR (wm IN [h,o] AND geo…)` AND `country NOT IN [..]` — one
group swapped, nothing else moved.

### "Apply my profile" sets the flag; the modal can change it

`filtersFromProfile` sets `workModeRemoteAnywhere` when the profile accepts remote
with no remote reach, accepts at least one physical mode, and states a base or
relocation geography (a stated reach keeps the plain flatten: the modifier would
drop it from the remote arm) —
the exact shape the location form collects, so the flag is the flattening of what
the user actually answered rather than a new question. The modal's Work format
pane gains a checkbox (shown only when remote AND a physical mode are both
included and some location facet is set — the same condition, live), serializing
`work_mode_remote_anywhere=true` and counting as one active filter when set. The
summary shows a chip so a shared link's behaviour is visible.

### Saved searches and alerts inherit the modifier

`savedSearchQuery` does NOT strip it: the digest matcher replays the stored query
through the same `filterFromValues`, so an alert mails the same set the subscriber
saw. This is the reason the modifier lives in the shared builder rather than in
the handler.
