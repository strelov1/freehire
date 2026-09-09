# 01 — How many questions each ATS puts on an application form

Measured against production (`hire`) on **2026-09-09**. Read-only.

The corpus is live: capture workers keep running, so every figure here is a snapshot of
that date and the article must say so.

## Query A — the corpus

```sql
SELECT provider, count(*) FROM apply_forms GROUP BY provider ORDER BY 2 DESC;
```

```
greenhouse|283363
lever|117424
ashby|87988
recruitee|79584
workable|79436
```

Total: **647,795**.

Moved since the design doc was written the same day (greenhouse 282,664 → 283,363,
recruitee 79,334 → 79,584; the other three unchanged). Not material to any claim, but it is
why the article dates its numbers instead of stating them flat.

## Query B — total fields per form, full corpus

Counts fields rather than reading `required`, which is the only measurement comparable
across all five platforms (see `02-required-fields.md` for why `required` is not).

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

```
  provider  | forms  | avg_fields | median_fields | max_fields 
------------+--------+------------+---------------+------------
 greenhouse | 283363 |       19.9 |            19 |        113
 lever      | 117424 |       14.6 |            15 |        154
 workable   |  79436 |       14.1 |            13 |         78
 ashby      |  87988 |       10.5 |            10 |         50
 recruitee  |  79584 |        8.5 |             7 |         60
(5 rows)

Time: 45599.345 ms (00:45.599)
```

**45.6 s** over 647,795 rows. `jsonb_array_length` reads the jsonb header rather than
unnesting, which is why this is cheap enough to run over the whole corpus; the required-field
pass in task 2 unnests and will not be.

## What the numbers support

**The mean is usable here, and that had to be checked rather than assumed.** Mean and median
sit within 1.5 of each other on every platform (19.9/19, 14.6/15, 14.1/13, 10.5/10, 8.5/7).
Had they diverged — a mean of 19.9 against a median of 6 — the average would have been
describing a handful of monster forms rather than what a candidate meets, and the article
would have had to lead with the median or drop the figure.

**The spread is the finding: 19.9 against 8.5, a factor of 2.3.** The same application costs
a candidate roughly twice as much work on Greenhouse as on Recruitee, and the candidate
never chose either — the employer did.

**The worst case is a Lever form with 154 fields.** Greenhouse's longest is 113. Note these
are *fields*, not required fields; how much of a form is mandatory is task 2's question, and
for Lever it is one this data cannot answer.

## What the numbers do not support

- Nothing about *why* a platform's forms are longer. Greenhouse's default template, the kind
  of employer that buys it, and the questions those employers add are three different
  explanations and this measurement separates none of them.
- Nothing about the labour market. This is five ATS platforms whose postings we crawl and
  could capture a form for — skewed toward tech, and toward companies large enough to buy an
  ATS at all.
- Nothing about how long a form takes to fill. A 19-field form of checkboxes is not a
  9-field form with three essay questions.
