# 02 — How much of an application form is mandatory, and why Lever is excluded

Measured against production (`hire`) on **2026-09-09**. Read-only.

## Query A — proving the Lever exclusion from the data

The design argued from code that Lever's `required` is not the same measurement as the
other four: Greenhouse, Ashby, Workable and Recruitee each read a boolean the platform's
API declares (`greenhouse.go:65,99`, `workable.go:18`, `recruitee.go:34`, Ashby's
`isRequired` at `fetch.go:207`), while Lever has no such field and infers requiredness from
a `✱` glyph appended to the label (`lever.go:14-17`) and an HTML `required` attribute
(`lever.go:210`).

An argument from code is not enough to publish on, so: what share of each platform's forms
report *zero* required fields?

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

```
  provider  | forms | pct_zero_required 
------------+-------+-------------------
 lever      |  2315 |              48.8
 recruitee  |  1582 |               0.0
 workable   |  1601 |               0.0
 ashby      |  1701 |               0.0
 greenhouse |  5717 |               0.0
(5 rows)

Time: 1974.453 ms (00:01.974)
```

**Conclusive, and not a matter of degree.** Nearly half of Lever's forms report no required
field at all; the other four report a form with none essentially never — 0.0% each, across
10,601 sampled forms.

An application form with zero mandatory fields is not a thing that exists. Every one of
them asks for a name or an address to reply to. So the 48.8% is not a fact about Lever's
generosity; it is the failure rate of reading requiredness out of markup instead of out of
an API. Whatever a Lever required-count would say, it would be measuring us, not them.

There is a second, independent reason, recorded in the design: `main` carries "Stop
refusing every Lever posting for a captcha most of them do not have" (#2721), so Lever's
117,424 rows were collected under a capture rule that was rejecting postings until very
recently. Either reason alone disqualifies the comparison.

**Therefore:** Lever is excluded from every required-field figure, and the article says why
in the body. Lever stays in the total-field comparison (`01-fields-per-form.md`), which
counts fields and never reads the flag.

## Query B — required fields on the four API-sourced platforms, full corpus

```sql
SELECT provider,
       count(*)             AS forms,
       round(avg(n_req), 1) AS avg_required,
       percentile_cont(0.5) WITHIN GROUP (ORDER BY n_req) AS median_required,
       max(n_req)           AS max_required
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

```
  provider  | forms  | avg_required | median_required | max_required 
------------+--------+--------------+-----------------+--------------
 greenhouse | 283363 |         13.2 |              13 |           76
 workable   |  79436 |         10.3 |               9 |           75
 ashby      |  87988 |          8.3 |               8 |           40
 recruitee  |  79584 |          6.8 |               5 |           48
(4 rows)

Time: 47008.202 ms (00:47.008)
```

**47.0 s** over 530,371 rows. This one unnests every form's field array, unlike
`01-fields-per-form.md`'s header read, and it is the most expensive query in this article's
work. Run on a host that is otherwise busy; one pass, not repeated.

## What the numbers support

**The mean is usable again, and again that was checked rather than assumed.** Mean and
median sit within 1.8 on every platform (13.2/13, 10.3/9, 8.3/8, 6.8/5).

**The 2% sample predicted the full corpus almost exactly** — sampled 13.5 / 10.3 / 8.2 /
6.8 against measured 13.2 / 10.3 / 8.3 / 6.8. Worth recording because it says the sampling
in the exploratory phase was not misleading us, and because the article's framing figures
should still be the full-pass ones.

**The worst case is 76 mandatory answers for one job application**, on Greenhouse; Workable
reaches 75. Against a median of 13, those are not typical — which is exactly why the median
travels beside them.

**Roughly two thirds of a form is mandatory**, and consistently so: 13.2 of 19.9 on
Greenhouse (66%), 10.3 of 14.1 on Workable (73%), 8.3 of 10.5 on Ashby (79%), 6.8 of 8.5 on
Recruitee (80%). The optional questions are the minority everywhere.

## What the numbers do not support

- Nothing about Lever's real mandatory count. We do not know it and the article must not
  imply we do.
- Nothing about *who* made a form long. A platform's default template and an employer's
  added questions are not separated by this measurement.
- Nothing about effort. 13 required checkboxes and 13 required essay boxes count the same
  here and are not the same afternoon.
