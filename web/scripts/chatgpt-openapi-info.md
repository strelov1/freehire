Public, read-only freehire API: search IT jobs, read the live filter
vocabulary, inspect job and company details, and find similar jobs. Every
operation here answers without a key; an API plan's key only raises the rate
limit. Keyless use is for personal and non-commercial use — showing the
postings in a product you offer to others needs an API plan (see the
[data licence](https://freehire.me/terms#data-licence)). The authenticated tracking surface
(saved jobs, application stages, CV tooling) is deliberately excluded — this
schema is the integration contract for search, and is also what the freehire
custom GPT imports as an Action.

## Filter grammar

Facet parameters are **repeatable**, and the same grammar applies to all of
them:

* `?skills=go&skills=kubernetes` — values of one facet are **ORed**: match
  either.
* `?skills=go&skills=kubernetes&skills_mode=and` — `<facet>_mode=and` flips
  that facet to **AND**: match both. Not available for geography (see below).
* `?source_exclude=adzuna` — `<facet>_exclude` removes matches. **Every**
  string facet in this schema declares its `_exclude` twin.
* Different facets **AND** together: `?work_mode=remote&seniority=senior` is
  remote *and* senior.

## Geography is one OR-group

`regions`, `countries`, `cities` and `metros` name a single concept — *where* — so
their values join into **one OR-group** instead of intersecting.
`?countries=gb&cities=London` therefore returns *United Kingdom **or**
London*, which is **wider** than `?countries=gb` alone — not narrower. To
search one place, name only that place.

Picking two places reading as "either" is what makes
`?regions=eu&countries=br` ("Europe or Brazil") useful. There is no AND to
switch on: `<facet>_mode=and` does not apply to these three.

`?regions=none` is a reserved value that selects postings with **no resolved
geography**, rather than a real region code.

## Resolve values before filtering

`skills`, `countries`, `cities`, `category`, `source`,
`company_slug` and `collections` are open or large vocabularies. Call
`getJobFacets` (or `searchCities` for a city) to discover canonical values
with live counts. The `cities` filter also accepts recognized dictionary
aliases such as `München` and resolves them to the canonical facet value;
`searchCities` remains useful for discovering or disambiguating a city.
Unknown values are not an error; they simply match nothing.

## API plans

The read API is available without a key for personal and non-commercial use.
An API plan licenses the data for use in your product — see the
[data licence](https://freehire.me/terms#data-licence).
A full-access API key (`fhk_…`) or OAuth access token (`fhm_…`) identifies
your account when sent as `Authorization: Bearer <token>`.

## Please say who you are

Not required, not validated, and nothing behaves differently without it —
every caller gets the same answers. It is a request.

Send a `User-Agent` naming your project:

```
acme/job-sync/1.4 (+https://github.com/acme/job-sync)
```

`owner/project` is the part that matters; the version and contact URL are
useful if you have them. What it buys you is a message before a limit or a
field changes, instead of a `429` as first notice — a default library
user-agent is indistinguishable from a thousand others, so there is nobody
to warn.

If identification ever gates anything, it will be through a key we issue,
not a string you set yourself.

## An unknown PARAMETER is dropped, and says so

A parameter no filter reads is ignored rather than refused, so old saved
searches and shared links keep working. The cost is that `?country=it` —
singular, and not the facet's name — returns the entire catalogue, which
looks exactly like a search that legitimately matched everything.

So the response tells you. `meta.ignored_params` lists what was thrown away,
with `did_you_mean` when the miss was only grammatical number:

```json
"meta": {
  "total": 1475149, "limit": 10, "offset": 0,
  "ignored_params": [
    { "param": "country", "did_you_mean": "countries" },
    { "param": "skil" }
  ]
}
```

**Check it before trusting a filtered result.** The key is absent on a clean
request, never `[]`, so its presence alone is the signal. `searchJobs`,
`agentSearchJobs`, `searchCompanies` and `getJobFacets` all report — and
`getJobFacets` grows a `meta` block it otherwise does not have. The report is
capped at ten entries.

`searchCompanies` checks against its OWN vocabulary: a jobs facet sent there
(`?seniority=senior`) is a real facet name, but not one company search reads,
so it is ignored and reported.

## An unknown VALUE of a closed facet narrows to nothing, and says so

Facets with a fixed vocabulary (`work_mode`, `regions`, `seniority`,
`category`, `employment_type`, `education_level`, `company_type`,
`company_size`, `salary_period`, `relocation`, `english_level`, `role_type`,
`source_type`, `ai_archetype`, `domains`, `reality`, `perks`, `engagement`, `skills` and the boolean facets) still
apply a value outside it — so `?work_mode=foo` answers `total: 0`, which reads
exactly like an empty market. `meta.ignored_values` names such values per
parameter (the `_exclude` form under its own name):

```json
"meta": { "total": 0, "limit": 10, "offset": 0,
          "ignored_values": { "work_mode": ["foo"] } }
```

Values are compared case-insensitively. A `skills` value is checked against
the skill dictionary that tags postings, so an alias it knows (`golang`) is
not unknown — it is read as its canonical slug (`go`). Open facets
(`countries`, `cities`, `company_slug`, `source`, `collections`,
`salary_currency`, `posting_language`) are not checked: an unknown value there can be
legitimate. Absent on a clean request; capped at ten parameters of ten
values each. Reported by `searchJobs`, `agentSearchJobs` and
`getJobFacets`.
