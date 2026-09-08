# Design

## What already exists

This change is mostly composition. Every piece it needs is written, tested and running —
what is missing is a route that reaches them without an account.

| Piece | Where | Already account-free? |
|---|---|---|
| PDF/text upload reading | `readResumeUpload` (resume.go:518-546) | Yes — no `requireUserID` inside it |
| Deterministic profile from CV text | `resumeProfile` (resume.go:192) | Yes — pure, no LLM, no I/O |
| ATS score | `atscheck.Score` (atscheck.go:136) | Yes — documented "pure and I/O-free" |
| Market coverage | `coverageFor` (resume_verdict.go:61) | Yes — reads only Meilisearch facets |
| IP rate limiting | `ratelimit.Middleware` + `KeyByIP` (handler.go:741) | Yes |

`resumeProfile` is a better role-inference than asking the visitor: it runs `skilltag.Parse`
over the whole text (skills appear anywhere) but `classify.Categories` over the *headline*
only — the current title plus the top of the summary — so a backend engineer who once
mentioned "marketing" in a bullet is not filed under marketing.

## Decisions

### One facet query feeds both halves

`deterministicReport` (ats_report.go:131) and `coverageFor` both open with the same call:
`FacetCounts({Filter: roleFilter, Facets: ["skills"]})`. The public handler issues it once
and passes the result to both, so the ATS score's `roleTopSkills` and the coverage reading
can never disagree about what the role demands. This is the same reason `cv_ats_delta.go`
routes both its sides through one `scoreRenderedCV`.

### The role is inferred, never guessed

`classify.Categories` "never guesses" — it returns empty when its dictionary resolves
nothing. Three states, all of which the page states plainly:

1. **Resolved** — `categories[0]` becomes the market filter. The page names it and offers
   a picker to change it.
2. **Nothing resolved** — no role filter. Coverage is measured against the whole open
   catalogue and the page says so, in those words. It does not pick a plausible role: a
   coverage figure attributed to the wrong role is worse than one attributed to no role,
   because the reader cannot tell it is wrong.
3. **Overridden** — the visitor picks a role; that filter wins over the inference.

The seniority `resumeProfile` resolves is *not* folded into the filter. A junior's coverage
against junior-only postings is a smaller, sadder number that says nothing about their
skills, and the reading this page sells is about skills.

### Nothing is stored, and no model is called

The uploaded bytes live in the request and are discarded with it — no S3 `Put`, no database
row, no cache entry. This is a deliberate difference from `ExtractResumeProfile`, which
stores because a signed-in user's later steps need the file; an anonymous visitor has no
later step here. The PII masking layer (`internal/candidate/pii`) is therefore not on this
path — it exists to protect a CV *from a model*, and no model is called.

Withholding the LLM review is not only about cost. It is the reason to create an account:
the deterministic score names *what* is wrong with the CV, and the model review is what
rewrites it. Giving away the second half leaves the page with no next step to offer.

`atscheck.NewAnalyzer(nil)` already makes `Analyze` a no-op returning `(nil, nil)`, so
"deterministic only" is the analyzer's own documented degradation rather than a new branch.

### Refusals and degradations

| Situation | Behaviour |
|---|---|
| Body over the server's 8MB `BodyLimit` | 413 from Fiber, before the handler |
| Not a PDF, or an undecodable one | 400 `invalid PDF` — the existing `readResumeUpload` wording |
| Scanned/image CV (< `minReadableWords` = 30 words) | 200 with a real report: `machine_readable` fails, which is the single biggest ATS killer. This is the most valuable answer the page gives, not an error |
| `classify` resolves no category | 200, whole-catalogue coverage, stated |
| Meilisearch unconfigured or unreachable | 200 with the ATS half only, and an explicit note that the market reading is unavailable. The ATS score needs no search — degrading to it beats a 503 |
| Rate limit exceeded | 429 |

The Meilisearch degradation is the one place this design adds a branch rather than reusing
one: `MarketCoverage` returns 503 when `h.facets == nil`, which is right for an API client
and wrong for a landing page whose other half still works.

### Rate limit

`ratelimit.Middleware(cfg.Throttler, ratelimit.KeyByIP("cvroast"), 10, time.Hour)`. Ten per
IP per hour. The work is a `pdftotext` subprocess plus three Meilisearch facet queries — not
free, but not a model call either; the limit is set to stop a scraper rather than to ration
a scarce resource. It is deliberately looser than a per-minute limit would be: a visitor
iterating on their CV genuinely re-uploads several times in a row, and that is the behaviour
the page wants.

### Where the code lives

A new `internal/api/handler/cv_roast.go` beside `ats_report.go` and `market_coverage.go`, on
`resumeHandlers` — it needs the same `h.facets`, and the helpers it composes are that
type's methods. The route registers in `resume.go`'s `register`, with no auth middleware and
the rate limiter, grouped with the existing public `POST /market/coverage` rather than the
`/me/profile/*` block, with a comment saying it is public on purpose.

Layering: `internal/api/handler` is layer 8 and already imports `candidate` (4), `search`
(5) and `dict` (2). No new block edge, so `internal/platform/arch/layering` needs no entry.

## The page

`web/src/routes/roast/+page.svelte` — public, no `/my` layout. One drop zone, then the
result:

```
Your CV: 62 / 100                        [ Re-run with AI review → sign in ]

  Keyword strength      22 / 40   ← the role's top skills you don't name
  Format compliance     18 / 20
  Section completeness  10 / 15   ← no Skills section
  Content quality        8 / 15
  Length & density       4 / 10

Read as: Backend engineer   [change]

Your skills reach 12,400 of 41,900 open backend postings.
Adding Kubernetes reaches 3,100 more.
```

The score and the breakdown come straight from `atscheck.Report`'s existing wire shape, so
the same `LineItem`/`Status` frontend component the signed-in `cv-readiness` page uses
renders both — `cmd/gen-contracts` already treats that shape as shared.

## What this does not do

- No article writing. That is the follow-on content task this page exists to serve.
- No change to `/me/profile/ats-report`, `/market/coverage`, or the `cv-readiness` page.
- No caching of results. Two uploads of the same file recompute; the work is small and a
  cache keyed on anonymous CV content is a store of CVs by another name.
