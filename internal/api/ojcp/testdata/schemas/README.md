# Vendored OJCP JSON Schemas

These are the published OJCP v0.1 schemas, copied verbatim from
[ojcp-org/ojcp](https://github.com/ojcp-org/ojcp) at commit
`8c8ac8f57950b97bb95d889ceb3db79b62a8f6a1` (2026-09-16, "add url and
official_job_url to JobPosting (RFC 0002 impl)").

They are the **oracle** every projection test in this package validates against: a
projection is correct when the standard's own schema accepts it, not when our test
author agrees with it. Hand-written Go assertions would drift from the spec silently;
these cannot, because they are the spec's own artifact.

**Do not edit them.** When OJCP publishes a change, re-copy the affected files and
update the commit hash above in the same diff — a stale hash is worse than none,
because it claims a provenance the bytes no longer have.

Only the schemas the read surface needs are vendored:

| File | Validates |
|---|---|
| `manifest.json` | the document served at `/.well-known/ojcp.json` |
| `job-posting.json` | one projected posting (referenced by `responses/job-detail.json`) |
| `responses/search-jobs.json` | the `search_jobs` response envelope |
| `responses/job-detail.json` | the `get_job_detail` response |
| `responses/employer-context.json` | the `get_employer_context` response |
| `responses/error.json` | the error envelope both transports render |

The schemas are JSON Schema draft 2020-12 and reference each other by absolute `$id`
URL (`https://ojcp.dev/schemas/v0.1/...`). The loader registers every vendored file
under its own `$id` before compiling, so resolution happens entirely offline — a test
run must never depend on ojcp.dev being reachable.
