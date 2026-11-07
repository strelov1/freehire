# Public search API

The JSON API that the web app, the CLI, the MCP servers and outside clients call:
search postings, read one posting by its slug, and get told which filters were not understood.

## Sub-features

- `search` answers `{data, meta}` with `meta.total` for a query.
- `job` returns one posting by `public_slug`; an unknown slug is 404.
- `ignored` reports unknown params (`meta.ignored_params`) and unknown closed-vocabulary
  values (`meta.ignored_values`) instead of silently widening.

## How to get to it (user POV)

- `GET /api/v1/jobs/search?q=<text>&limit=<n>` (and the facet filters in `web/static/openapi.yaml`).
- `GET /api/v1/jobs/<slug>`.
- Same paths through the web origin (`$VERIFY_WEB_URL/api/...`), which is what the browser uses.

## Driving it with Playwright

The API needs no browser: curl it. Preconditions: baseline.

- **Search.** `curl -s "$VERIFY_API_URL/api/v1/jobs/search?q=engineer&limit=2"`: `meta.total` > 0, titles match.
- **Job.** Take a `public_slug` from that answer; `curl -s "$VERIFY_API_URL/api/v1/jobs/<slug>"`
  returns `data.title`. `curl -s -o /dev/null -w '%{http_code}' "$VERIFY_API_URL/api/v1/jobs/no-such-job-xxxxxxxx"` is `404`.
- **Ignored.** `...search?q=engineer&colour=red` has `meta.ignored_params: [{"param":"colour"}]`;
  `...search?work_mode=foo` has `meta.ignored_values: {"work_mode":["foo"]}`.
- **Proof.** `| tee "$VERIFY_EVIDENCE/api-<step>.json"` on each call.

## Gotchas

- `/api/v1/jobs` (no `/search`) is a different, unfiltered listing; filters go to `/search`.
- A change to the wire shape must also pass `scripts/check-openapi-breaking.sh`; this map
  proves behaviour, not the contract.
