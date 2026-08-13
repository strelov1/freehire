# Anonymous sharing of tracker application-process data — design pass

Design pass for [#1625](https://github.com/strelov1/freehire/issues/1625). No implementation
is proposed yet; this resolves the open questions the issue lists and recommends a scope.

**Recommendation in one line:** do not build this at the posting level. Build it at the
**role cluster**, reuse the gates `company-hiring-signal` already ships, and **measure the
tracker distribution before writing any of it** — the measurement may show the gate is never
met, which is a cheap way to find out.

## Context

The issue proposes aggregating `internal/userjob` tracking data onto a job posting:
applicant counts, stage-to-stage timing, outcome distribution, time-to-first-response.

The framing to correct first: **freehire already ships this one level up.** The
`company-hiring-signal` capability maintains a per-company application response rate and a
median time-to-first-reply, both counted from `application_events`, and it already answers
most of what the issue lists as open:

| Question the issue raises | Already decided in `company-hiring-signal` |
|---|---|
| Sample-size floor | **10 tracked applications.** Below it the field is *absent* — not zero, not an estimate |
| Which rows count | Only applications whose owner has a **connected mailbox**, on both sides of every ratio |
| Source of truth | `application_events`, never live `emails` rows — a candidate clearing old mail must not make an employer look silent |
| Censoring | Unanswered applications are excluded from the median and reported as a count beside it |
| Corrections | Retracted events drop out of every aggregate |

So the real question is not "how do we aggregate tracking data" — that is a solved,
shipped problem — but **"does the company-level design survive being pushed down to a
single posting?"**

It does not. That is the finding this document exists to record.

## Why the posting level is categorically harder, not just smaller

### 1. The adversary knows the input set

k-anonymity assumes an adversary who cannot enumerate the population. On a job posting, the
adversary who matters — the employer running it — **holds the exact list of everyone who
applied**, in their own ATS. The aggregate is computed over a subset of a list they already
have.

A company-level rate pools across many postings, many roles and years of history, so
"27% response rate at Acme" reveals nothing about any individual. "5 people are tracking this
posting, 2 were rejected" tells Acme's recruiter that 5 of their 40 named applicants use
freehire, and gives them two outcomes to match against their own records. The floor of N ≥ 5
the issue floats does nothing here, because the protection k-anonymity offers was never
against this adversary.

### 2. A live counter leaks by differencing, and a static floor cannot stop it

This is the argument that decides the design, and it is the one a "N ≥ 5" threshold misses
entirely.

The aggregate updates as applications arrive and stages move. An employer who loads the
posting page daily observes the *increments*, not just the level:

- Tuesday: they send a rejection to one named candidate.
- Wednesday: the posting's `rejected` count moves 2 → 3.

They have now learned that this specific person tracks their application on freehire, and
confirmed the outcome against a named individual. Nothing about a publication floor prevents
it: the floor gates *whether the aggregate is shown*, and the leak is in *how it moves once
shown*. Raising the floor to 10, or 50, changes nothing — an adversary who can observe
differences on a set they control can attribute every increment.

Any design here must therefore make the published value **insensitive to a single
contributor's arrival or state change**, which is a much stronger requirement than a minimum N.

### 3. Small-N medians are close to per-person disclosure

A median over five observations *is* one person's value — the third one. "Median 12 days
applied → interview" over a 5-person sample publishes an individual's timeline under a word
that sounds aggregate. Timing statistics need a substantially higher floor than counts do,
and the issue treats them as one question.

### 4. Stage timing is mostly not computable today, by deliberate design

`appevent.TrustedForDayMath` already answers the issue's "users don't always log every stage"
question, and the answer is stronger than the issue assumes. Only mail- and calendar-sourced
events carry a timestamp somebody other than the candidate set. `stage_set` is written from the
`user`, `assistant` and `system` sources — **none** of which `TrustedForDayMath` admits — with
the reasoning stated at the definition:

> A manually-recorded stage dates from when the candidate got around to updating their board,
> so a funnel built on it would measure diligence and report it as market behaviour.

The ledger spec says the same in a scenario titled *"Stage velocity starts empty and honest"*.
So "typical time between stages", the issue's second bullet, is not a presentation problem
waiting on an aggregation layer — the inputs are deliberately untrusted. It becomes available
only for transitions evidenced by mail or calendar (`employer_reply`, `interview_scheduled`),
which is a much narrower feature than "applied → screen → interview → offer".

## Resolutions to the issue's open questions

**Anonymity floor.** Reuse **10**, the gate `company-hiring-signal` already uses for
rate-shaped aggregates. Do not invent 5. Two gates exist in this codebase for two different
purposes and neither is 5: 10 tracked applications for a published rate, and
`ghost.ContributorGate = 2` for *distinct people* behind an outcome claim about a posting — the
latter carrying the note that a served count of one deanonymises that applicant to the employer.
A third number would be a third vocabulary for one idea, which is the mistake `internal/userjob`
already recorded when it deleted `buckets.go` rather than deprecate it.

**Count distinct users, not applications.** At cluster grain one person can track several
postings of the same role across cities. The `(user_id, job_id)` primary key makes one row per
user per *posting*, not per cluster, so a naive count would let a single diligent tracker clear
the gate alone. This is also the cheapest gaming vector.

**Opt-in vs opt-out.** Account-level, **default off**, one setting — not per tracked job. Two
reasons. Per-job consent asks the candidate to decide before they know the outcome, and they
will revisit it after: people share an offer and quietly withhold a rejection, so per-job
consent manufactures exactly the selection bias that destroys an outcome distribution. And
opt-in of any kind already biases the denominator — an opted-in cohort is not a random sample of
applicants — so whatever ships must describe itself as "among freehire users who opted in",
never as a market rate. Note the asymmetry with the company rate, which ships with no opt-in at
all and is defensible because its aggregate is over a scale where no individual is visible.

**What is shown.** Buckets, never raw counts, and buckets with **hysteresis** — a band must be
crossed by a margin before the display changes, so no single arrival or stage change can move
what is rendered. Combined with a **cohort freeze** (aggregate only over applications whose
`occurred_at` is older than a closed window, e.g. 30 days) this is what actually answers the
differencing attack: the published value stops tracking this week's decisions, and the employer
watching the widget sees nothing correlated with what they did on Tuesday. A floor alone does not
buy this; the freeze and the hysteresis do.

**Where it is not applicable.** Anywhere the gate is unmet — and the honest expectation is that
this is nearly everywhere at posting grain. The company rate is already documented as
"expected to be absent for nearly every company until the sample matures; that absence is the
correct answer, not an unfinished implementation." A posting-level version inherits that with a
far smaller denominator.

**Staleness and reposts.** Aggregating at the **role cluster** answers this rather than
patching it. freehire already clusters postings on `role_fingerprint` (which excludes location)
and marks non-canonical copies `duplicate_of`; the reality signal already reports `RepostCount`
and `MassPostingCount` off that cluster, and the company rollup already excludes
`duplicate_of` rows so counts match `companies.job_count`. Aggregating per cluster means a
repost is the same subject as the posting it repeats, so the aggregate follows the *role* and
does not reset every time an employer re-posts — while simultaneously raising N, which is the
one change that helps every problem above at once.

The grain is already a first-class one in the schema rather than something this feature would
introduce: `migrations/0003_role_fingerprint.sql` creates `jobs_company_role_fingerprint_idx` on
`(company_slug, role_fingerprint)` precisely to back the two per-cluster counts the reality
signal serves. The aggregate proposed here would group on the same key, behind the same index.

**Abuse and gaming.** The connected-mailbox requirement is the structural defence and should be
kept for both correctness and abuse: an attacker cannot fabricate an `employer_reply` they have
no mail for. What remains reachable is **denominator pollution** — fake tracked applications
with no mail, which would *deflate* an employer's apparent response rate. Mitigations, in order
of cost: count distinct users (above), require a verified email, and cap contribution per user
per cluster to one. `internal/companyfeedback`'s per-window cap plus report/hide is the
precedent for user-contributed content, but it is aimed at authored text; here there is no text
and the attack is on the counter, so the rate limit matters less than the identity gates.

## Recommended scope

**Do not build a posting-level feature.** Extend the existing company rollup to a
`(company_slug, role)` grain, which is the same machinery, the same gates, and the same
worker (`cmd/rollup-company`, already an atomic delete-and-reinsert in one transaction).

Everything else is deferred: stage-to-stage funnels (inputs untrusted), applicant counts on a
posting page (differencing), per-job consent (selection bias), and the free-text interview
reviews discussed in the issue's comment, which are a separate feature over
`internal/companyfeedback` and share none of this machinery.

## Do this first, before any implementation

One read-only measurement decides whether the rest is worth designing further:

> Over applications whose owner has a connected mailbox, what is the distribution of
> **distinct users per role cluster**, and how many clusters reach 10?

The `observable` CTE below is lifted verbatim from `RebuildInsightsCompanyResponse`
(`internal/db/queries/insights.sql`), so the measurement is taken under exactly the gate the
feature would ship with — including the reason a calendar-only Google grant does not count.

```sql
-- Read-only. Distribution of distinct observable trackers per role cluster.
WITH observable AS (
    SELECT ae.user_id, ae.job_id
      FROM application_events ae
     WHERE ae.kind = 'applied'
       AND ae.retracted_at IS NULL
       AND ae.job_id IS NOT NULL
       AND (EXISTS (SELECT 1 FROM gmail_connections gc
                     WHERE gc.user_id = ae.user_id AND gc.status = 'connected' AND gc.email <> '')
         OR EXISTS (SELECT 1 FROM mailboxes mb WHERE mb.user_id = ae.user_id))
), clusters AS (
    SELECT j.company_slug,
           j.role_fingerprint,
           count(DISTINCT o.user_id) AS trackers
      FROM observable o
      JOIN jobs j ON j.id = o.job_id
     WHERE j.company_slug <> ''
       AND coalesce(j.role_fingerprint, '') <> ''
     GROUP BY j.company_slug, j.role_fingerprint
)
SELECT trackers,
       count(*)                                        AS cluster_count,
       count(DISTINCT company_slug)                    AS companies
  FROM clusters
 GROUP BY trackers
 ORDER BY trackers DESC;
```

Read it as a histogram: the rows at `trackers >= 10` are the entire addressable surface of this
feature today. If that is a handful of clusters concentrated at one employer, the feature does
not exist yet regardless of how it is designed, and the correct outcome is to close the issue as
premature and revisit when the tracker base has grown. That is a query, not a project, and it
should be run before anything else here is scheduled.

Note the join is on `application_events.job_id`, which `cmd/prune` sets to NULL — so pruned
postings drop out and the figure is a slight **under**-count. That is the right direction for a
go/no-go measurement, but it means the query cannot double as the feature's own aggregation,
which would have to pair through `application_id` the way the company rollup does and cluster
via the denormalised `company_slug`.

## Open questions this pass did not settle

- **Bucket boundaries and hysteresis width** need the distribution above to choose sensibly.
  Picking them first would be a guess presented as a threshold.
- **Whether the cohort freeze window is 30 days** or longer depends on how quickly a cluster
  accumulates trackers — too long and the aggregate describes a hiring round that has closed.
- **Round-aware stages** (the issue's comment) would sharpen stage timing, but only for
  transitions that mail or calendar evidence can date. It does not change the trust rule, so it
  is a smaller unlock than it looks.
- **Whether opting in should be retroactive** over a user's existing applications, or apply only
  going forward. Retroactive is more useful and is a consent question, not a technical one.

## Related

- `openspec/specs/company-hiring-signal/spec.md` — the shipped company-level aggregate and its gates
- `openspec/specs/application-event-ledger/spec.md` — what the ledger guarantees and what it refuses to backfill
- `internal/appevent/appevent.go` — `TrustedForDayMath`, the trust rule
- `internal/userjob/AGENTS.md` — stages, the silence ladder, why the ledger and not the columns feed aggregates
- `internal/ghost/AGENTS.md` — `ContributorGate = 2`, the existing per-posting anonymity gate
- `internal/companyfeedback/companyfeedback.go` — rate limiting and report/hide for user-contributed content
