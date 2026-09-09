## Why

The ATS-readiness scorer is one of the strongest things the product does, and nobody who
has not already signed up can reach it. `atscheck.Score` is a pure, I/O-free function and
`MarketCoverage` describes itself as "the stateless, API-key sibling of the CV-based
verdict" — both halves of a useful answer already need no account — yet the only doors are
`GET`/`POST /me/profile/ats-report` behind `mw.cookie` and `POST /market/coverage` behind
`mw.key`. The single UI is `web/src/routes/my/profile/cv-readiness/`, under `/my`.

That closed door is what makes the search-traffic channel unbuildable. The queries this
product should own — "why does the ATS reject my resume", "ATS resume format", "resume
keywords" — are asked by people who have never heard of us, and the honest landing page for
such a query is one that answers it in ten seconds. Sending them to a sign-up form instead
spends the visit to gain nothing. `/jobs/find` already established the counter-pattern in
this codebase: the useful thing is public and first, the account comes later.

There is also something here nobody else can copy. An ATS score is commodity arithmetic —
any résumé tool computes one. "Your skills reach 12,400 of the open postings for this role;
adding Kubernetes reaches 3,100 more" requires a live catalogue of millions of postings,
which is the asset this product already has and a résumé-builder competitor does not.

## What Changes

- Add a public page, `/roast` ("Roast my CV"), that takes an uploaded CV and returns a
  deterministic ATS score with its per-category breakdown, plus a market-coverage reading
  of the skills it found — with no account, no stored file, and no model call.
- Add the public endpoint behind it, `POST /cv/roast`, which extracts the PDF's text,
  tags its skills, infers the role from the text, and computes both readings from one
  facet query. It is IP-rate-limited through the existing `ratelimit.Middleware` and it
  writes nothing anywhere.
- Infer the role from the CV rather than asking for it: `classify.Categories(cvText)`
  yields the categories a résumé spans in precedence order, and its first entry becomes the
  market filter. The page names the inferred role and offers a picker to override it. When
  the dictionary resolves nothing, the page measures against the whole catalogue and says
  so rather than guessing a role.
- Keep the paid and expensive half behind the account exactly where it is: the LLM content
  review, CV tailoring, and anything that persists. The public page ends on a CTA into
  them.

Out of scope for this change: the articles that will link here (a content task, not a code
one), any change to `/me/profile/ats-report`, and any storage of an uploaded file.

## Capabilities

### New Capabilities

- `public-cv-roast`: the account-free CV assessment — what an anonymous visitor may upload,
  what the page tells them (ATS score, per-category breakdown, market coverage, the single
  highest-yield missing skill), how the role is inferred and overridden, what is refused,
  and the boundary between what is free and what requires an account.

### Modified Capabilities

None. The existing signed-in surfaces keep their current behaviour; this change adds a
second, narrower door onto the same scoring functions.
