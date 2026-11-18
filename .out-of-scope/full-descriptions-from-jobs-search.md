# Full descriptions from `/api/v1/jobs/search`

This project keeps the `description` in `/api/v1/jobs/search` results a preview. Full descriptions
come from `GET /api/v1/agent/jobs/search`, a search endpoint that returns each description whole
(`description_format`), or from `GET /api/v1/jobs/{slug}` for one posting. What each endpoint
accepts is in [docs/API.md](../docs/API.md).

## Why this is out of scope

- `/jobs/search` is served straight from Meilisearch, and the indexed description is capped at
  1000 runes (`maxIndexedDescriptionRunes`, `internal/search/search/document.go`). The inverted
  index over `description` dominates the index size, and a full rebuild has to fit in the host's
  free disk beside the live index. Descriptions average about 4900 runes; the first 1000 hold the
  role summary and the leading requirements, where matches land.
- A `full_description` flag here would be the first of many one-off shaping flags on the endpoint
  the website depends on; a separate endpoint keeps that path fast and simple.

Revisit if the index stops carrying the description at all, or its size stops being the
constraint on a rebuild.

## Prior requests

- #1104: "api/v1/jobs/search description field is truncated" (answered by the agent endpoint)
