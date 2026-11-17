# Knowledge update history

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
