# 04 — How fast the skill-coverage curve flattens

Measured against production (`hire`) on **2026-09-09**. Read-only.

For the second article to land traffic on `/roast`. The question: adding a skill to a CV
opens some number of additional postings — how does that number fall off?

## Method, and the denominator that matters

Every figure is against **open, tech-classified postings in one category** that carry at
least one tagged skill. The last clause is not decoration: `internal/job/verdict`'s own
comment gives the reason — "skill frequency is measured against this, not the raw role
total, so postings the tagger left skill-less don't deflate frequencies." For backend that
is 41,712 of 45,453 open postings (91.8%); measuring against 45,453 would report the
tagger's coverage as if it were the market's.

Coverage uses PostgreSQL's array-overlap operator (`skills && ARRAY[...]`), so a posting
counts once no matter how many of the listed skills it names. That is the whole point:
frequencies add up to more than 100% and unions do not, which is why "api appears in 51% of
postings" cannot be read as "knowing api reaches 51%".

Each query took 50–65 s. The host is saturated (a bare `count(*)` on this predicate takes
57 s), so these were run one at a time rather than widened.

## Query A — top skills, open backend postings

```sql
SELECT s, count(*) FROM jobs, unnest(skills) s
WHERE closed_at IS NULL AND is_tech IS TRUE AND category = 'backend'
GROUP BY 1 ORDER BY 2 DESC LIMIT 25;
```

```
api|23167
java|17964
cloud|17096
aws|13378
ci-cd|13289
ai|12681
python|11962
sql|10742
docker|10476
kubernetes|10283
spring|10166
microservices|9710
postgresql|9538
rest|9531
agile|8014
git|7491
kafka|7422
distributed-systems|6624
devops|6507
azure|6506
nodejs|5846
gcp|5294
observability|5257
typescript|4861
nosql|4710
Time: 61051.529 ms (01:01.052)
```

Worth noting for a future piece rather than this one: `ai` sits at 12,681 in *backend*
postings — above sql, docker and kubernetes.

## Query B — the coverage curve, backend

```sql
SELECT count(*) AS all_open,
       count(*) FILTER (WHERE cardinality(skills) > 0) AS with_skills,
       count(*) FILTER (WHERE skills && ARRAY['api']) AS top1,
       count(*) FILTER (WHERE skills && ARRAY['api','java','cloud']) AS top3,
       count(*) FILTER (WHERE skills && ARRAY['api','java','cloud','aws','ci-cd']) AS top5,
       count(*) FILTER (WHERE skills && ARRAY['api','java','cloud','aws','ci-cd','ai','python','sql','docker','kubernetes']) AS top10
FROM jobs
WHERE closed_at IS NULL AND is_tech IS TRUE AND category = 'backend';
```

```
 all_open | with_skills | top1  | top3  | top5  | top10 
----------+-------------+-------+-------+-------+-------
    45453 |       41712 | 23168 | 34198 | 35958 | 38742
(1 row)

Time: 64644.523 ms (01:04.645)
```

Against 41,712 skill-bearing postings: **55.5% → 82.0% → 86.2% → 92.9%**.

The second and third skills buy 26.5 points. The fourth and fifth buy 4.2. The sixth
through tenth buy 6.7 between them.

## Query C — the same shape in three other categories

Top five per category first:

```sql
WITH r AS (
  SELECT category, s, count(*) n,
         row_number() OVER (PARTITION BY category ORDER BY count(*) DESC) rn
  FROM jobs, unnest(skills) s
  WHERE closed_at IS NULL AND is_tech IS TRUE
    AND category IN ('frontend','data','devops','mobile')
  GROUP BY 1, 2
)
SELECT category, string_agg(s, ', ' ORDER BY rn) AS top5 FROM r WHERE rn <= 5 GROUP BY 1;
```

```
 category |                     top5                     
----------+----------------------------------------------
 devops   | cloud, devops, ci-cd, kubernetes, automation
 frontend | react, typescript, api, css, javascript
 mobile   | android, ios, api, kotlin, swift
(3 rows)

Time: 50247.030 ms (00:50.247)
```

`data` returned no rows — it is not a category slug this catalogue uses. Not chased; four
categories is enough to answer whether the shape is a backend accident.

Then the curve, each category against its own top skills:

```
 category | with_skills | top1  | top3  | top5  
----------+-------------+-------+-------+-------
 devops   |       74060 | 45836 | 60051 | 62363
 frontend |       15361 | 10310 | 12774 | 13655
 mobile   |       15425 |  9341 | 12737 | 12845
(3 rows)

Time: 51156.894 ms (00:51.157)
```

## The finding

| category | 1 skill | 3 skills | 5 skills |
| --- | --- | --- | --- |
| backend | 55.5% | **82.0%** | 86.2% |
| devops | 61.9% | **81.1%** | 84.2% |
| frontend | 67.1% | **83.2%** | 88.9% |
| mobile | 60.6% | **82.6%** | 83.3% |

**Three skills reach 81–83% of every category measured.** Four independent fields, entirely
different skills in each, and the three-skill figure lands inside a two-point band.

The flattening is sharpest in mobile: three skills to five adds **0.7 points**. Two more
skills, less than one percent of the market.

## What this does not support

- **It is not advice about which three skills.** The top three differ per field and shift
  with the market; the finding is the shape of the curve, not its contents.
- **Coverage is not a shortlist.** A posting "reached" by a skill still asks for everything
  else on it. This measures which postings you are legible to, not which you would get.
- **The tagger's vocabulary bounds it.** `internal/dict/skilltag` is dictionary-only and
  emits nothing for a term it does not know, so a skill absent from the dictionary is absent
  from these counts — that is why the denominator is skill-bearing postings.
- **Categories are dictionary-resolved from titles.** A posting whose title resolves to no
  category is not in any of these four populations.
- **One employer can be many postings.** A company with 3,000 open roles pushes its stack up
  these counts. These are per-posting figures, not per-employer ones.
