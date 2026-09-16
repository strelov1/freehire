# Vendored OJCP JSON Schemas

These are the published OJCP v0.1 schemas, copied verbatim from
[ojcp-org/ojcp](https://github.com/ojcp-org/ojcp) at commit
`8c8ac8f57950b97bb95d889ceb3db79b62a8f6a1` (2026-09-16, "add url and
official_job_url to JobPosting (RFC 0002 impl)").

They are the **oracle** every projection test in this package validates against.
Hand-written Go assertions drift from the spec silently; these cannot, because they are
the spec's own artifact.

**What the oracle proves is narrower than "the projection is correct", in two ways that
matter when writing the projection tests.** Do not read a green schema check as more than
it is:

1. `job-posting.json` does not set `additionalProperties: false`. A posting carrying
   `skills_requred` (misspelt) or `applyPaths` (wrong casing) validates clean — the field
   is simply invisible to the schema. A projection that drops `skills_required` and
   `apply_paths` entirely is schema-conforming and useless, so **presence of the fields we
   mean to emit needs its own assertion**, beside the schema check.
2. `responses/search-jobs.json` **inlines** a laxer job-summary shape instead of `$ref`-ing
   `job-posting.json`. Validating a `search_jobs` envelope therefore does NOT validate its
   `jobs[]` elements as JobPostings. Each element must be validated against
   `job-posting.json` separately, or the per-posting projection goes untested on the
   busiest surface of all.

Tightening either by editing these files would be wrong — they are the spec's artifact,
not ours. These are limits to know, not to fix.

**Do not edit them.** When OJCP publishes a change, re-copy the affected files and
update the commit hash above in the same diff — a stale hash is worse than none,
because it claims a provenance the bytes no longer have.

Only the schemas the read surface needs are vendored — the three read tools' inputs and
responses, the posting shape both responses carry, the manifest, and the error envelope:

| File | Validates |
|---|---|
| `manifest.json` | the document served at `/.well-known/ojcp.json` |
| `job-posting.json` | one projected posting (referenced by `responses/job-detail.json`) |
| `tools/search-jobs-input.json` | what an agent may send to `search_jobs` |
| `tools/get-job-detail-input.json` | what an agent may send to `get_job_detail` |
| `tools/get-employer-context-input.json` | what an agent may send to `get_employer_context` |
| `responses/search-jobs.json` | the `search_jobs` response envelope |
| `responses/job-detail.json` | the `get_job_detail` response |
| `responses/employer-context.json` | the `get_employer_context` response |
| `responses/error.json` | the error envelope both transports render |
| `candidate-context.json` | `$ref`-ed by `tools/search-jobs-input.json` |

`candidate-context.json` is here only because the search input `$ref`s it. This surface is
anonymous and accepts no candidate PII — the read tools ignore the field. It is vendored
so the input schema compiles at all, which is how the refusing loader surfaced it: without
that loader the compile would have fetched it from ojcp.dev and nobody would have known.

The input schemas are here for the same reason the response ones are: the tool handlers
must read what the standard says an agent may send, and "ignore what you do not recognise"
is only checkable against the list of what IS recognised.

The schemas are JSON Schema draft 2020-12 and reference each other by absolute `$id`
URL (`https://ojcp.dev/schemas/v0.1/...`). The loader registers every vendored file
under its own `$id` and hands the compiler a loader that **refuses every URL**, so
resolution is offline by construction rather than by the library's default: a `$ref` this
tree does not satisfy fails the compile naming the missing `$id`, instead of quietly
fetching it and making CI depend on ojcp.dev being up.

The loader also turns on `format` assertion, which draft 2020-12 leaves off by default.
Without it a `datePosted` of `16/09/2026` and a `url` of `not a url` both validate clean.
