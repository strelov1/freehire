## Why

Members who track an application leave a dated record of what happened to it: when they applied,
when the employer answered, whether the answer was a rejection or an invitation. Today that record
reaches nobody but its owner, except through two channels that show how thin it is:

- `insights_company_response` — a per-COMPANY reply rate and median time to first reply, served
  on the company page once a company has 10 observable applications. It is expected to be absent
  for nearly every company.
- `jobs.applied_count` — an exact count of members who marked a posting applied, already public.

A person deciding whether to spend an evening on a tailored application would like the same
signal about THIS role: do people hear back, how long does it take, how often is it a no. Blind and
Levels.fyi do this for compensation; freehire already holds the raw material for process timing
and needs no new ingestion.

The hard part is not the aggregation, it is that the unit asked for (one posting) is almost never
large enough to publish. On 2026-10-31 the whole tracker held 11,856 applications against ~890k
open postings, and trackers concentrate on a few popular ones. A per-posting figure behind any
k-anonymity floor will be absent on nearly every posting page, and an opt-in shrinks the pool
further. This change therefore designs the feature around that constraint instead of discovering
it after the build.

## What Changes

This is a DESIGN change. It adds no code. It records the decisions below so an implementation can
be picked up without a second round of product and privacy debate, and it names the one
measurement that decides whether to build at all.

- **Unit of aggregation: the role cluster**, not the posting row — the same duplicate closure
  `GET /jobs/:slug/copies` lists, so reposts and per-city copies pool, and a rolling 180-day
  window, so a figure describes how the role is hiring now.
- **One vote per member per cluster**, counted as distinct members, never applications.
- **Opt-in, account level, default off**, with a per-application "keep out of the aggregate"
  switch. Revoking takes effect at the next rollup; nothing per-member is stored in the aggregate.
- **Outcomes only from evidence an employer produced**: replies read from mail the platform
  fetched itself, and calendar interviews. A stage the member typed never enters the aggregate,
  and neither does `mail_external` (pushed through an API key, so forgeable).
- **Published, quantized, and gated**: nothing below 10 distinct contributors; contributor count
  as a band, shares to the nearest 10%, time to first reply as a rounded range gated separately
  at 5 answered. Offers and acceptances are never separated out.
- **Served from a daily rollup**, as its own endpoint (`GET /jobs/:slug/process-signal`), not as a
  field of the job wire shape. Rows below the floor are not written, so absence cannot be told from
  "fewer than ten".
- **A go/no-go measurement first**: a hand-run read-only report of what share of job-page VIEWS
  would land on a cluster clearing the floor, under 100%, 50% and 20% opt-in.
- **A fallback ladder** when the cluster is below the floor: the existing company-level figure,
  and nothing else. No invented intermediate grouping.

Open for the owner at review (each has a recommendation in `design.md`): whether the existing
company rate should move under the same consent, whether to surface that existing rate on the job
page now, and whether auto-apply submissions count.

## Capabilities

### New Capabilities

- `application-process-signal`: the consent, the aggregation unit, the publication rules and the
  serving contract for an anonymized, per-role view of how applying has gone for members.

## Impact

None until implemented. The implementation touches `internal/application` (a new
`processsignal` package, added to the layering table in `internal/platform/arch/layering/blocks.go`),
`cmd/rollup-company` (one more pass), `internal/api/handler` and `web/static/openapi.yaml`
(additive endpoint), a migration for the consent and the per-application switch, the job page, the
account settings page and the privacy policy. Ops: none beyond the existing rollup timer.
