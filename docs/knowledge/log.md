# Knowledge update history

## 2026-11-22

Documented LinkedIn "firehose" searches: Board `*` omits the `keywords` parameter so one search
per location lists the whole market, and `tenants.jobage_hours` (1..720, overrides `jobage`)
sets sub-day windows. Evidence is a live guest-API measurement: keyword-less
`location=Germany&f_TPR=r86400&f_AL=true` returns cards, and listing depth tops out around
600-700 cards per query (start=700 empty for Germany, and once for the US, whose 24h Easy Apply
flow exceeds it), so big markets need windows of a few hours. `harvest-linkedin` gained
`--jobage-hours` and a per-entry `jobage_hours`. No production rollout is asserted.

## 2026-11-17

Documented the optional LinkedIn Easy Apply listing pass (`tenants.easyapply_pages`). Evidence is a
live guest-API measurement today: 6 of 6 Easy Apply cards had an on-site apply control against 1 of
8 without the filter; the detail page still decides publication. No production rollout is asserted.

Documented the LinkedIn non-native read cache (`linkedin_postings_read`): external and unknown
postings are re-read only after the body-refresh window. Evidence is the implementation and its
tests; no production rollout is asserted.

Documented that the LinkedIn adapter re-reads a stored posting only every `BODY_REFRESH_DAYS`
through the seen-set (provider-wide, keyed by bare posting id), and that a liveness touch never
reopens a `native_source_pending` row. Evidence is the implementation and its tests; the
`ingest-env/linkedin.env` setting is applied separately.

## 2026-11-16

Documented independent WantApply global/Cyprus inventories and the original-postings
exception to company-wide coverage. Evidence is the adapter, coverage implementation,
and regression tests; production recovery is checked separately.

Linked assistant merge consent and requirement-report ownership to their current package
references. Evidence is the local working tree and regression tests, not a production rollout.

Documented native ATS ownership for external LinkedIn discoveries, identity-preserving
transfer, board discovery and scheduler ownership. Evidence is the implementation and
public Gist HRsmart sample; deployment is verified separately.

## 2026-11-15

Linked the catalogue concept to the WhatJobs summary-matching implementation and
its worker reference. Evidence is the local source and captured public postings;
no production deployment is asserted.

## 2026-11-14

Created the OKF v0.2 navigation bundle from repository instructions and package
references. Concepts include pinned source provenance and agent authorship;
production state and human verification have not been asserted.

## LinkedIn source identity and related discovery — 2026-11-16

Recorded numeric posting identity with legacy id/history preservation, the closed-history
URL lookup index, and bounded first-level related-job discovery. Configured Scrapling
routing retains the source's request budget. This describes the checkout implementation;
production rollout is verified separately.

## Recurring LinkedIn harvest — 2026-11-16

Added provenance for registering the existing hiring-market worklist into the normal
board catalogue and for bounded per-search page/date options. Runtime yield is measured
separately; the worklist is not evidence of complete LinkedIn coverage.

## Concurrent LinkedIn posting writes — 2026-11-16

Recorded the transaction-scoped gate for each native LinkedIn posting before legacy identity
lookup and writes. A production-candidate integration run exposed a competing public-slug
unique check before the ordinary upsert conflict branch. This is a write correctness rule;
provider request pacing and unrelated posting writes keep their existing behavior.

## LinkedIn intake and related discovery — 2026-11-16

Updated catalogue provenance for shared LinkedIn link intake, contributor attribution,
numeric posting identity with legacy-row preservation, and bounded first-level related
job discovery. Configured API fetch routing changes only LinkedIn; arbitrary ATS URL
resolvers keep their guarded transport. This describes the checkout implementation, not a
production rollout.

Verified native intake preserves regional board identity and contributor attribution on
vanity posting hosts. A recorded board adopts its provider even when the exact posting
read fails, while operator pauses remain intact. Evidence is the focused handler
integration suite; production verification is separate.

Preserved independently verified native employer metadata across a later posting-read
failure. The regression covers pending-board naming and a successful native retry with
an adapter that labels jobs from its board entry; no unverified posting is published.

## LinkedIn throttle retries — 2026-11-17

The first 80-search harvest run (2026-11-17) logged ten detail 429s in 25 minutes through the
Scrapling proxy; each ended its search after 7-64 of ~100 listed postings while neighbouring
requests were served. The adapter now retries a 429 twice after growing pauses before treating
it as a refusal. 403/999 and a persistent 429 still stop the board; pacing is unchanged.

## Vendor hosts from LinkedIn employer discovery — 2026-11-17

The first 80-search LinkedIn harvest onboarded three vendor addresses as employer boards from
career-page links: iCIMS's cookie-script host, Recruitee's analytics and careers_not_hosted
fallback hosts, and SmartRecruiters' candidate portal path. Each was probed (404, empty, redirect,
0 postings); the recognizer now reserves them. qa.recruitee.com is a live tenant and stays.

## LinkedIn request interval — 2026-11-17

The LinkedIn limiter's 2s spacing became `LINKEDIN_REQUEST_INTERVAL` (default unchanged,
100ms..10s, invalid fails the crawl). Requests already leave through the rotating proxy with a
new exit IP each, and 158 searches after the 429 retry met no refusal at 2s, so the per-process
spacing, not LinkedIn, was the throughput bound. Ops lowers it in steps.

## Korean and Vietnamese title dictionaries — 2026-11-22

Recorded the title dictionary and role picker alias generator in the
[catalogue concept](catalogue.md#title-classification).

Vietnamese role categories now reach CV matching through the rendered text. A focused scorer
regression covers exact titles and equivalent roles. Korean course and Vietnamese teaching
and recruiting titles retain their exclusions from technical-job search.
