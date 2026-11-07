# A file export of the job tracker (XML, CSV or a JSON download)

This project serves the tracker as JSON and offers no separate download. `GET /api/v1/me/tracking`
(cookie or a full-scope API key) returns every tracked posting with its stage, notes and the dates
it was viewed, saved and applied to, paged; the account MCP server's `my_jobs` tool returns the
same rows to an agent.

## Why this is out of scope

- The data is already machine-readable through both routes, so a file button would be a second
  shape of the same rows to keep in step with the first.
- The only request came from the MCP `report_problem` tool on 2026-11-04 (two reports, both
  marked by the reporting agent itself as review-only tests), not from a person who needed it.

Revisit if people ask for a download they can keep or move to another tracker, or if an import
format another service accepts gives a file a reader the JSON API does not have.

## Prior requests

- `problem_reports` 1 and 2 (2026-11-04): "export my tracked job board to XML", test reports
- #4249 discussion (2026-11-07): declined in favour of the existing API and `my_jobs`
