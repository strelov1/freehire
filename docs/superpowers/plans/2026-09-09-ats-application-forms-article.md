# What application forms actually ask — implementation plan

**Goal:** Publish one evidence-based article at `/blog`, measured from the 646,846 captured
application forms, that gives search traffic a reason to arrive and `/roast` a reason to be
clicked.

**Architecture:** No code. Three measurement passes against production's `apply_forms`,
each saved raw before anything is rounded into prose, then one `.svx` post written from
what the measurements actually support.

**Spec:** `docs/superpowers/specs/2026-09-09-ats-application-forms-article-design.md` — read
it first; it is the requirements, and most of it is the traps.

## How "test-first" applies here

There is no production code and nothing to make fail. The analogue that carries the same
discipline: **a number is a claim, and its query plus raw output is the test that backs it.**

- A figure that reaches the prose without saved raw output is an unbacked claim — the
  equivalent of code with no test — and does not ship.
- A figure that disagrees with what the code says the data means is a broken measurement,
  not a finding. Stop and diagnose before writing a sentence around it.
- The measurements come before the prose. Writing first and measuring after is how the
  numbers end up serving the sentence instead of the other way round.

## Global constraints

- **English only** in the article, the commit messages and the saved measurements.
- **Production is read-only here.** `SELECT` only. Bound every query — the host is
  saturated, and `BACKFILL_CONCURRENCY=6` has degraded it before. One query at a time,
  timed; if a full pass is slow, say so rather than running it wider.
- **Raw output is saved before rounding**, into `docs/superpowers/plans/measurements/` —
  each file naming the exact query and the date it ran.
- Every number in the prose traces to a saved file. No exceptions, including ones that
  "obviously" follow.
- The corpus's shape is stated in the article, not assumed by it.
- Lever is excluded from required-field counts, and the article says why.
- No competitor product named as a villain; the five platforms are named as the corpus.

---

### Task 1: The cross-platform measurement (total questions per form)

This is the article's headline claim and the only one that is comparable across all five
platforms, because it counts fields rather than reading a `required` flag that means
different things in different places.

**Files:**
- Create: `docs/superpowers/plans/measurements/01-fields-per-form.md`

- [ ] **Step 1: Confirm the corpus is what the spec says it is**

Re-run the count that the spec quotes, so the article's own framing number is current
rather than remembered:

```sql
SELECT provider, count(*) FROM apply_forms GROUP BY provider ORDER BY 2 DESC;
```

Save the output verbatim. If any figure has moved materially from the spec's table
(282,664 / 117,424 / 87,988 / 79,436 / 79,334), update the spec too — the article must not
quote a number the spec contradicts.

- [ ] **Step 2: Measure total fields per form, full corpus**

```sql
SELECT provider,
       count(*)                                              AS forms,
       round(avg(jsonb_array_length(payload -> 'fields')), 1) AS avg_fields,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY jsonb_array_length(payload -> 'fields')) AS median_fields,
       max(jsonb_array_length(payload -> 'fields'))           AS max_fields
FROM apply_forms
GROUP BY provider
ORDER BY avg_fields DESC;
```

Time it. `jsonb_array_length` reads the jsonb header rather than unnesting, so this should
be far cheaper than the sampled query that unnested — but confirm rather than assume.

- [ ] **Step 3: Report the median beside the mean**

A single 400-field form drags a mean and tells the reader nothing about what they will
meet. If mean and median diverge sharply for any platform, the article leads with the
median and says why. Record which one each platform's figures support.

- [ ] **Step 4: Save the measurement**

Write `01-fields-per-form.md`: the exact SQL, the date, the raw output pasted unedited, the
wall-clock time, and one line on anything surprising.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/measurements/01-fields-per-form.md
git commit -m "measure: how many questions each ATS puts on an application form"
```

---

### Task 2: The required-field measurement (four platforms, and the exclusion)

**Files:**
- Create: `docs/superpowers/plans/measurements/02-required-fields.md`

- [ ] **Step 1: Prove the exclusion before relying on it**

The spec asserts Lever's `required` is not the same measurement. Before the article says so
in public, confirm it from the data as well as from the code:

```sql
SELECT provider,
       count(*) AS forms,
       round(100.0 * count(*) FILTER (WHERE n_req = 0) / count(*), 1) AS pct_zero_required
FROM (
  SELECT provider,
         (SELECT count(*) FROM jsonb_array_elements(payload -> 'fields') f
          WHERE (f ->> 'required')::bool) AS n_req
  FROM apply_forms TABLESAMPLE SYSTEM (2)
) t
GROUP BY provider
ORDER BY pct_zero_required DESC;
```

The expectation from reading `internal/ingest/applyform/lever.go:14-17,210`: Lever shows a
far higher share of forms with zero required fields than the four API-sourced platforms,
because a glyph-and-attribute heuristic catches less than a declared boolean.

**If the data contradicts that expectation, stop.** Either the code reading is wrong or the
exclusion is unjustified, and the article cannot claim either until it is settled.

- [ ] **Step 2: Measure required fields on the four API-sourced platforms**

```sql
SELECT provider,
       count(*)          AS forms,
       round(avg(n_req), 1) AS avg_required,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY n_req) AS median_required,
       max(n_req)        AS max_required
FROM (
  SELECT provider,
         (SELECT count(*) FROM jsonb_array_elements(payload -> 'fields') f
          WHERE (f ->> 'required')::bool) AS n_req
  FROM apply_forms
  WHERE provider <> 'lever'
) t
GROUP BY provider
ORDER BY avg_required DESC;
```

This one unnests, so it is the expensive pass. Time it and report the time. If it runs
long, note it — a slow query on a saturated host is worth saying out loud, not hiding.

- [ ] **Step 3: Save the measurement**

Write `02-required-fields.md`: both queries, the date, raw output for each, timings, and an
explicit paragraph on what the Lever check showed and what the article is therefore
entitled to say.

- [ ] **Step 4: Commit**

```bash
git add docs/superpowers/plans/measurements/02-required-fields.md
git commit -m "measure: how much of an application form is mandatory, and why Lever is out"
```

---

### Task 3: What they actually ask (and disclosing how it was grouped)

The section no competitor can reproduce, and the one with the subtlest trap: labels are
stored verbatim, so counting them raw splits one question across many spellings.

**Files:**
- Create: `docs/superpowers/plans/measurements/03-question-labels.md`

- [ ] **Step 1: Look at the raw labels before deciding how to count them**

```sql
SELECT lower(trim(f ->> 'label')) AS label, count(*) AS n
FROM apply_forms TABLESAMPLE SYSTEM (2), jsonb_array_elements(payload -> 'fields') f
WHERE coalesce(f ->> 'label', '') <> ''
GROUP BY 1
ORDER BY n DESC
LIMIT 200;
```

Read the 200 rows before choosing a grouping. The point of looking first is to find out how
badly the same question fragments — that answer decides the method, and guessing the method
before looking is how the section becomes decoration.

- [ ] **Step 2: Choose and record the grouping method**

Pick exactly one, and write down why in the measurement file:
- exact labels (honest, fragmented, probably useless as a table);
- a stated normalisation (case, punctuation, whitespace — say precisely what);
- hand-grouped families, listing every raw label folded into each family.

The article must disclose whichever was used and show enough raw labels for a reader to
check it. **Do not present a frequency table whose grouping is invisible** — that is the
same defect as an unsourced percentage, which this article exists to criticise.

- [ ] **Step 3: Run the chosen count on the full corpus, excluding the standard fields**

Name, email and CV are on every form; they are the floor, not the finding. Exclude them by
the identifiers our own mappers produce (see `internal/ingest/applyform/display.go:51-90`
for how the display path already folds them into one `basics` line) rather than by matching
label text, which is exactly the fragmentation this task is guarding against.

- [ ] **Step 4: Save the measurement**

Write `03-question-labels.md`: the exploratory query and its raw 200 rows, the grouping
decision with its reasoning, the final query, and its raw output.

- [ ] **Step 5: Commit**

```bash
git add docs/superpowers/plans/measurements/03-question-labels.md
git commit -m "measure: what an application form asks beyond name, email and CV"
```

---

### Task 4: Write the article

**Files:**
- Create: `web/src/posts/2026-09-09-what-application-forms-actually-ask.svx`

- [ ] **Step 1: Read three existing articles first**

Read `web/src/posts/2026-07-20-what-78-ats-apis-taught-us.svx`,
`2026-07-14-why-we-publish-our-numbers.svx` and
`2026-08-07-chasing-a-stuck-reindex.svx`. Match their register and their frontmatter
exactly. This blog states the measurement first and puts the caveat beside the number
rather than saving it for the end; write the same way rather than in a house style this
post would be the only example of.

- [ ] **Step 2: Write it from the saved measurements only**

Structure per the spec: the size of the ask; how much is mandatory (four platforms, and why
Lever is out); what gets asked beyond the basics; the worst cases; what this does and does
not prove.

Every figure comes from a file in `measurements/`. If a sentence wants a number no
measurement produced, either measure it or cut the sentence — do not estimate.

Address the unsourced "70%" by saying it is unsourced. Do not replace it with a figure of
ours that would be equally unsourced.

- [ ] **Step 3: Close on `/roast`**

The article explains what these systems read; the page tells the reader what it reads in
their own CV. One link, in the reader's interest, not a banner.

- [ ] **Step 4: Verify it renders and nothing else broke**

```bash
pnpm --dir web test
pnpm --dir web lint
pnpm --dir web check
pnpm --dir web run build
```

Then run the app and open `/blog` and the post itself. `pnpm check:links` covers the
relative links in the repo's Markdown; the post's own links are checked by opening it.

- [ ] **Step 5: Trace every number back**

Before committing, walk the finished prose and point each figure at the measurement file it
came from. A figure that cannot be traced is cut. This is the step that makes the article
different from what it criticises, so it is not optional and not a skim.

- [ ] **Step 6: Commit**

```bash
git add web/src/posts/2026-09-09-what-application-forms-actually-ask.svx
git commit -m "post: what 646,846 application forms actually ask"
```

---

## Done means

- Three measurement files exist, each carrying its query, its date and its unedited raw
  output.
- Every figure in the article traces to one of them.
- The Lever exclusion is proven from the data, not only argued from the code.
- The label table discloses its grouping.
- `pnpm --dir web test`, `lint`, `check` and `build` all pass, and the post renders.

## Deliberately not here

- The other three articles the angle supports. One ships first because no article has
  earned an impression yet.
- Any link to `/roast` from the nav or the footer. That is a product placement decision
  nobody has made; the sitemap entry already carries discoverability.
