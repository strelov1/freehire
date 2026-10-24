## Context

`HeaderSearch.svelte` holds a draft, fetches completions/postings/companies for it, and on a
pick applies an `ApplyPlan` (`facets`, `q`, `qFields`) through `target.suggest.applyParts`.
The job search API already accepts `company_slug`, `source` and `q_fields=title`.

## Goals / Non-Goals

**Goals:** the three operators, translated into existing filters on Enter; contextual
dropdown after an operator's colon.

**Non-Goals:** server-side parsing of `q` (API/CLI/MCP keep the explicit params); negation
(`-company:`); operators on the companies list page; more keys.

## Decisions

### Parse in the browser, apply as existing filters

The operators are a typing shortcut for filters that already exist. Translating them in the
box keeps one representation in the URL, saved searches and digests (`company_slug=…`), so
nothing downstream learns a second spelling of the same filter.

### Parser grammar

`key:value` where `key` is one of `title`, `company`, `source` (case-insensitive), at the
start of the text or after whitespace, and `value` is a run of non-space characters or a
`"…"` phrase (an unclosed quote runs to the end). Any other `word:` — `c++:`, `node.js:`,
`https://` — stays ordinary text. An empty value is dropped. A key may repeat: two
`company:` values are two values of the facet (OR, as the facet already behaves); a second
`title:` joins the first into one title phrase.

### Company value resolution — exact, through the companies endpoint

A company facet needs a slug. The slug rule (`normalize.CompanySlug`) and its legal-form
vocabulary live in Go, and AGENTS.md forbids a second copy of that vocabulary. So the box
resolves a typed name by asking `/api/v1/companies?q=<value>` and taking the item whose
name or slug equals the value case-insensitively. A value picked from the contextual
dropdown already carries its slug and skips the request.

An unresolved value is NOT guessed, and it stops the whole commit: nothing applies, the
caret moves to that value, and the panel shows its options — or says nothing matches. A
feed filtered by half of what was typed is not what was typed. When no company name starts
with the value, the endpoint's own fuzzy order is offered, since here the visitor has said
they are picking a company (`company:"Google Inc."` shows Google).

### Source value resolution — against the live source distribution

`loadSourceDistribution()` (already used by the avoid-sources picker) is fetched once,
lazily, the first time a `source:` token appears. A value matches a source key or its
`sourceLabel` case-insensitively. An unresolved value stops the commit, same rule as companies.

### `title:` makes the whole remaining text title-only

Meilisearch applies `attributesToSearchOn` to the whole query, so `title:Backend rust`
cannot search "Backend" in titles and "rust" everywhere in one request. With a `title:`
present, the title phrase and any free text form one quoted `q` with `qFields=['title']` —
the same plan a picked title suggestion produces today.

### Contextual dropdown

The token under the caret decides the mode. Inside a `company:` value the panel shows only
company rows (from the companies endpoint, prefix-filtered by `namedCompanies`); inside
`source:` only source rows (filtered locally); inside `title:` only title completions from
`/api/v1/suggest`. Picking a row replaces that token with `key:"Canonical Name" ` and keeps
focus, so the visitor can keep typing. Outside any operator the dropdown is unchanged, and
the text sent to `/suggest`, job search and companies has the operators stripped.

## Risks / Trade-offs

- Exact company matching misses "Google Inc." when the company is named "Google" →
  mitigated by the contextual dropdown, which is the expected path; the unresolved token is
  left visible rather than silently dropped.
- A visitor searching literally for `title:` as text loses that → acceptable; the three
  keys are not words people search for.
