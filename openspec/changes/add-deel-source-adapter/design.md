# Design — `deel` source adapter

## Context

`jobs.deel.com/<orgSlug>` is a Next.js App-Router app. The board page is server-rendered and
inlines the entire job catalogue for that org in the React **flight** stream — a sequence of
`self.__next_f.push([1,"<chunk>"])` calls whose JS-string bodies concatenate into one payload.
Within that payload:

- A component row carries the page props, including
  `"careerPageSettings":{ … "preferredOrganizationName":"Klarna" … }` and
  `"jobPostings":[ {posting}, … ]`.
- Each posting object looks like:
  ```json
  {
    "id": "5d3636f8-…",                    // posting id — used in the public URL + sitemap
    "jobId": "461d452c-…",                 // underlying job id (not used)
    "title": "Customer Trust … (Sweden)",
    "richtextDescription": "$23",          // reference to a text row in the same payload
    "createdAt": "2025-11-04T15:57:06.816Z",
    "job": {
      "jobLocations": [ { "location": { "name": "Sweden" } } ],
      "jobEmploymentTypes": [ … ], "currentCompensation": null, …
    }
  }
  ```
- The `"$23"` reference resolves to a **text row** elsewhere in the stream:
  `23:T1aef,<…HTML…>` — `T` marks a text chunk, `1aef` is the **byte** length (hex) of the HTML
  that immediately follows the comma.
- **Row ids are lowercase HEX, not decimal.** A board's references run `…$28, $29, $2a, $2b
  … $30 …`, so the row matcher must accept `a–f` digits. (Live-smoke caught this: a
  decimal-only matcher silently resolved only ~60% of a real board's descriptions — every
  posting whose id contained `a–f` came through with an empty body.)

Deel exposes **no** structured workplace-type / remote field on a posting (verified: zero
`workplaceType`/`remote`/`onsite` keys in a real board payload). Work mode is therefore left to
the downstream location parser; the adapter only sets the `Remote` flag from the shared
`isRemote` heuristic.

## Parsing algorithm

```
Fetch(board):
  html  = httpClient.Get("https://jobs.deel.com/<board>")
  flight = concat( jsonUnescape(chunk) for chunk in regex `self.__next_f.push\(\[1,"(…)"\]\)` )
  refs   = map[string]string             // "23" -> resolved HTML
      for each match of `(?m)(\d+):T([0-9a-f]+),` in flight:
          start = end-of-match
          refs[id] = flight[start : start + parseHex(len)]   // BYTE slice
  settings    = locate+decode the "careerPageSettings" object  → preferredOrganizationName
  postings    = locate+decode the "jobPostings" array          → []posting
  for each posting:
      desc = posting.richtextDescription
      if desc starts with "$": desc = refs[ desc[1:] ]        // resolve reference
      yield Job{ …mapping…, Description: sanitizeHTML(desc) }
```

Decoding details that matter:

- **JS-string unescape per chunk**: wrap each captured chunk body in quotes and
  `json.Unmarshal` it into a Go string. JSON string escaping (`\"`, `\\`, `\n`, `\uXXXX`,
  `\/`) is exactly the flight escaping, and it yields correct UTF-8 — the decode must not
  mangle multibyte characters (e.g. Klarna's "world's").
- **Byte-length slicing**: the `T<hexlen>` length counts bytes of the UTF-8 stream, so the
  resolved text is `flight[start : start+n]` on the byte string — not a rune count.
- **Locate-then-bracket-match**: find `"jobPostings":` / `"careerPageSettings":` and
  bracket-match the `[ … ]` / `{ … }` that follows, then `json.Unmarshal` that slice into a
  typed struct. Only the fields the adapter needs are modeled; everything else is ignored.
- The postings array is itself valid JSON (the `$N` references are ordinary JSON strings), so
  no special pre-processing of the array is required before unmarshalling.

## Why embedded-payload parse (not the JSON API)

The board data originates from `api-prod.letsdeel.com` (a `rest/v2` API that answers a
structured JSON 404 on wrong paths), but the call is made **server-side** in a React Server
Component, so the exact public path is not present in any client chunk and probing did not
recover it. The board page already embeds the complete catalogue with full descriptions, so the
embedded parse needs exactly one request per board and is fully sufficient. Discovering and
switching to the JSON API is recorded as a future hardening option, not a blocker. This mirrors
the `google` adapter, whose public JSON API is likewise gone and whose list pages inline the
payload.

## Decisions

- **`ExternalID` = posting `id`** (not `jobId`): it is the UUID in both the public job URL and
  the org sitemap, i.e. the canonical public addressing unit, keeping `external_id`, URL, and
  sitemap consistent.
- **Board = org URL slug** (e.g. `klarna`): the public, human-readable tenant key; the adapter
  is board-based and appears in the source facet (not boardless).
- **Fixture = trimmed real capture** (~2 postings + their text rows + `careerPageSettings`),
  matching the `google` adapter's testdata convention, so the brittle parse is pinned against
  real-shaped flight data including a real `$N`→`T<len>` resolution.

## Alternatives considered

- **Per-job detail fetch** (sitemap → `/<slug>/job-details/<id>/overview`): rejected — the
  board page already carries every description inline, so detail fetches would be pure N+1
  overhead.
- **Headless capture of the JSON API**: deferred — higher cost, and unnecessary while the
  embedded payload is complete. Left as the hardening seam above.
