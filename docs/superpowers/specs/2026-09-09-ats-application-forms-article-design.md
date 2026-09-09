# What 646,846 application forms actually ask — article design

## Why this article, and why this one first

`/roast` shipped on 2026-09-09 (PR #2712): a public, account-free page that scores an
uploaded CV against ATS rules and the live vacancy market. It exists to give search traffic
somewhere to land, and nothing links to it yet. This is the first article written to do that
job.

The competitive read that chose the angle. The SERP for "free ATS resume checker" is a crowd
of interchangeable tools — LiveCareer, MyPerfectResume, Resume Worded, Jobscan, SkillSyncer,
TripleTen, Freesumes, 1MillionResume. The SERP for "why does the ATS reject my resume" is
near-duplicate content marketing from résumé vendors; one site carries three variants of the
same piece. The widely repeated "70% of resumes are rejected before a human sees them"
travels without a source.

We can write the one thing none of them can: what these systems actually ask, measured from
646,846 captured application forms. That is also the voice this blog already has — each of
its ten `type: article` posts is "we measured X, and here is what it showed."

## The evidence

`apply_forms` on production, counted 2026-09-09: 646,846 rows, one per posting, `payload`
jsonb holding the platform's form verbatim.

| provider | forms |
|---|---|
| greenhouse | 282,664 |
| lever | 117,424 |
| ashby | 87,988 |
| workable | 79,436 |
| recruitee | 79,334 |

Each field carries `id`, `label`, `type`, `raw_type`, `required`. Platform vocabulary is
stored verbatim by design (`internal/ingest/applyform/AGENTS.md`), so the labels are the
employers' own words rather than our interpretation of them — which is what makes the
"what do they actually ask" section possible at all.

## The measurement trap this design exists to avoid

A 2% sample gave average required-field counts of Greenhouse 13.5, Workable 10.3, Ashby 8.2,
Recruitee 6.8, **Lever 2.2**. That last figure is not a finding about Lever, for two
independent reasons.

**One: it is not the same measurement.** Greenhouse, Ashby, Workable and Recruitee each read
`required` as a boolean the platform's own API declares (`greenhouse.go:65,99`,
`workable.go:18`, `recruitee.go:34`, Ashby's `isRequired` at `fetch.go:207`). Lever has no
such API field — its form is scraped from HTML, and requiredness is inferred two ways: a `✱`
glyph appended to the label (`lever.go:14-17`) and an HTML `required` attribute
(`lever.go:210`). Comparing the counts compares an API boolean against a markup heuristic.

**Two: the Lever corpus was built under a different rule.** `main` as of 2026-09-09 carries
"Stop refusing every Lever posting for a captcha most of them do not have" (#2721) — Lever
capture was rejecting postings until that landed, so its 117,424 rows are not the same kind
of sample as the other four.

Either reason alone disqualifies the comparison. The number would be wrong in exactly the way
this article criticises others for being wrong.

**Rule for this article:** required-field counts cover the four API-sourced platforms only,
and the article states why Lever is excluded. Total field counts
(`jsonb_array_length(payload -> 'fields')`) are comparable across all five and carry the
cross-platform claim instead.

## What the article says

1. **The size of the ask, per platform** — total questions on a form, all five. The headline:
   the same job costs a candidate very different amounts of work depending on which system
   the employer bought.
2. **How much of it is mandatory** — the four API-sourced platforms, with Lever's exclusion
   explained in the body rather than buried in a footnote. The exclusion is part of the
   story: it is what makes the other four numbers worth trusting.
3. **What gets asked beyond name, email and CV** — the most common question labels, in the
   employers' own words. The section no competitor can reproduce.

   This section carries its own measurement trap, and the same rule applies. Labels are
   stored verbatim, so counting them raw fragments one question across many spellings —
   "Are you authorized to work in the US?" and "Are you legally authorized to work in the
   United States?" are one question to a reader and two rows to a `GROUP BY`. Any grouping
   is therefore a judgement, and this article must **disclose the one it used** (exact
   labels, a normalisation, or hand-grouped families) and show enough raw labels for a
   reader to check it. A frequency table whose grouping is invisible is the same defect as
   an unsourced percentage.
4. **The worst cases** — the longest forms in the corpus, and what they ask for.
5. **What this does and does not prove** — the constraints below, stated in the piece.

The close hands the reader `/roast`: the article explains what these systems read; the page
tells them what it reads in *their* CV.

## Honesty constraints

Non-negotiable, because the article's entire claim to attention is that it is measured.

- **Every number is reproducible.** Each query and its raw output is saved with the work, not
  just the rounded figure that reaches the prose.
- **Full corpus, not the 2% sample.** The sample decided whether the article was writable.
  Published figures come from a full pass.
- **State the corpus's shape.** These are postings from five ATS platforms that we crawl and
  could capture a form for — not "all jobs", not a random sample of the labour market.
  Employers on these platforms skew toward tech and toward companies large enough to buy an
  ATS.
- **No invented causation.** "Greenhouse forms carry more questions" is a measurement.
  "Greenhouse makes hiring worse" is not, and does not appear.
- **The 70% claim** is addressed by saying it is unsourced — not by substituting a number of
  ours that would be equally unsourced.
- **Name platforms as the corpus, not as villains.** Naming the five is naming what was
  measured; a comparison against a named rival product is not this article's business.

## Shape and placement

- `web/src/posts/2026-09-09-what-application-forms-actually-ask.svx`, `type: article`. The
  slug carries the question a reader would type, not the corpus size — a number in a URL
  ages the moment the next capture run finishes.
- English, matching the existing articles' register: plain sentences, the measurement first,
  the caveat beside the number rather than saved for the end.
- Frontmatter exactly as the existing posts use it — `title`, `date`, `summary`, `type`,
  `tags`, `draft`. `web/src/lib/blogPosts.ts` is the only reader; the blog index, RSS,
  sitemap and OG image all follow from the file existing.
- No code change, and no OpenSpec change: this adds no capability.

## Out of scope

The other three articles this angle supports — ATS ranks rather than rejects; what a machine
genuinely cannot read; the skills gap that costs a candidate N postings. One ships first
because no article has earned an impression yet, and four articles into an unproven angle is
four times the mistake.
