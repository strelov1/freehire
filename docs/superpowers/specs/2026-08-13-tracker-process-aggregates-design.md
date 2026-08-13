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
that sounds aggregate. Timing statistics need a floor of their own, over their own
denominator, and the issue treats them as one question with the counts.

The codebase already agrees, and has for longer than the spec admits:
`responseSampleGate = 10` gates the rate over *observable* applications while
`replySampleGate = 5` gates the median over *answered* ones
(`internal/handler/company_response.go`), with the reasoning stated at the constant — "a
company can clear the first comfortably while the second rests on three data points. They are
two numbers with two denominators and they need two gates."

Note that 5 is the sample size this section calls unsafe. That was a defensible judgement at
company grain; it is not at cluster grain, where the employer knows the applicant set. The
*shape* is what this design inherits — two gates over two denominators — not the number, which
needs re-examining upward here.

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

**Anonymity floor.** Reuse the **pair**, never a single number: `responseSampleGate = 10` over
observable applications, `replySampleGate = 5` over answered ones. Inventing a third threshold
would be a third vocabulary for one idea, which is the mistake `internal/userjob` already
recorded when it deleted `buckets.go` rather than deprecate it — and a third gate already exists
for a third question, `ghost.ContributorGate = 2` over *distinct people* behind an outcome claim
about a posting, carrying the note that a served count of one deanonymises that applicant to the
employer.

The timing gate additionally counts only **day-math-trusted** observations, per §4: a transition
dated by `stage_set` is untrusted however many of them there are, so trusted-and-answered is the
denominator the floor applies to, not answered alone. Both numbers were set at company grain and
should be revisited upward here (§3).

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

**Consent semantics.** Opt-in is **retroactive** over the candidate's existing applications.
Prospective-only consent would leave the feature with no data for as long as an observation
window plus a cohort, which in practice means it launches empty; and the candidate is consenting
to a use of facts about their own history, not to a collection that has not happened yet.

Revocation follows the precedent already in the tree: `ghost_reports.retracted_at` withdraws a
report from the signal while keeping the row, so that "a retraction cannot be used to file
repeatedly", and `application_events` uses the same shape. Opting out therefore excludes a
contribution from future computations without deleting anything.

That leaves one edge worth naming, because it arrives from the consent side of the same attack
§2 describes: if revocation removed contributions from already-published windows, a cluster could
fall back below its gate and the aggregate would **un-publish** — and the disappearance is itself
informative, since it says somebody who was in this cluster left. The immutable-snapshot rule in
*What is shown*, below, absorbs it: a published window is never recomputed, so a revocation takes
effect from the next window rather than retracting a number already shown.

The same rule bounds the retroactivity above, and the two resolutions have to be read together:
consent reaches back over a candidate's existing applications, but only as far as **windows that
are still open**. It cannot enter a window already published, because that would let a consent
change move a number already shown — the same door §2 closes, approached from the other side.
Retroactive therefore means "every application whose window has not yet closed", not "everything
they have ever done", and the difference is worth stating to whoever writes the consent copy.

**What is shown.** Buckets, never raw counts, and buckets with **hysteresis** — a band must be
crossed by a margin before the display changes, so no single arrival or state change can move
what is rendered.

**Freeze the metric inputs, not cohort eligibility.** An earlier draft of this section proposed
admitting only applications whose `applied` event is older than 30 days. That does not defend
anything: it freezes *membership*, while the value that moves is the `employer_reply` — which is
the event the employer controls, and the one §2's attack is built on. An application admitted at
day 40 still flips from unanswered to answered the moment a rejection lands, so the employer acts
on Tuesday and the bucket moves on Wednesday exactly as before. The filter froze the one thing
the adversary does not control and left free the one thing they do.

Three channels mutate a cohort frozen that way: a late reply (employer-controlled — the attack),
a retraction from a link correction, and the bulk historical import that fires when a candidate
connects a mailbox and back-dates a year of replies into closed windows at once.

What actually closes them is a fixed **observation window** per application — admit it only once
that window has elapsed, compute at close, and never recompute. This reframes the metric rather
than filtering it: the published quantity has one correct value forever, instead of a "response
rate" whose value drifts and whose drift is the leak. The reframing is what makes immutability
honest — without it, freezing merely discards late replies and makes slow employers read as
non-responders, the distortion `company-hiring-signal` exists to remove.

**Immutability and completeness genuinely conflict, and the window alone does not resolve it.**
A window closes on `occurred_at`, but what can be *published* is bounded by `recorded_at` — the
distinction the ledger already draws ("Events distinguish when they happened from when they were
learned"). A reply that happened inside the window but is learned of after close is excluded
permanently, so a metric called "replied within N days" would overstate what was measured.

The channel that makes this severe rather than marginal is mailbox connection, which is a step
change rather than a drip. It also moves more than replies: `observable` is evaluated when the
rollup runs, so a candidate who applies before connecting a mailbox is not admitted to their
window at all, and connecting later cannot retroactively admit them once that window has closed.
The cohort would then be biased toward people who connected a mailbox *before* applying — the
minority, since the common path is to apply, get no answer, and connect a mailbox to use the
tracking features. `cmd/rollup-company` avoids all of this by rebuilding everything on every run;
that is exactly the property the snapshot rule removes, so the cost has to be paid somewhere else.

**Pay it by stamping observability at write time.** Admit only applications whose owner already
had a connected mailbox when the application was recorded — the same move as denormalising
`company_slug` and `role_fingerprint`, deciding at write what would otherwise be re-derived at
read. A window is then genuinely complete at close, because every reply it could ever contain
arrives through a mailbox that was already connected, and a later connection governs only which
*future* applications count instead of rewriting history.

That leaves bounded lag — mail sync, late classification, a link confirmed a week afterwards —
which a **grace watermark** before close absorbs. `sources.SweepGraceWindows` is the existing
shape: a provider whose observation is structurally incomplete gets a wider window rather than a
wrong verdict. Days, now that the bulk channel is gone.

With both in place the metric can be named for what it measures: **"replied within N days, among
applications observable when made."** The cohort restriction belongs in the name, because it is
the part a reader would otherwise assume away.

`internal/userjob` offers `terminalStages`/`IsTerminal` as an alternative admission rule, but a
settled stage is candidate-recorded, so gating on it would bias the cohort toward diligent
trackers. A fixed window does not.

The cost, which should be decided rather than inherited: once a window is published, a mislinked
reply inside it can no longer be repaired by retraction. The mail stack cares about exactly this
— one catalogue company sharing an ATS's name once collected twenty-three acknowledgements
belonging to other employers. Under immutable snapshots, correcting that becomes a deliberate
operator republish, and a republish is itself a value change.

**Where it is not applicable.** Anywhere the gate is unmet — and the honest expectation is that
this is nearly everywhere at posting grain. The company rate is already documented as
"expected to be absent for nearly every company until the sample matures; that absence is the
correct answer, not an unfinished implementation." A posting-level version inherits that with a
far smaller denominator.

**Staleness and reposts.** Aggregating at the **role cluster** answers this rather than
patching it. freehire already clusters postings on `role_fingerprint` (which excludes location)
and marks non-canonical copies `duplicate_of`; the reality signal already reports `RepostCount`
and `MassPostingCount` off that cluster. Aggregating per cluster means a repost is the same
subject as the posting it repeats, so the aggregate follows the *role* and does not reset every
time an employer re-posts — while simultaneously raising N, which is the one change that helps
every problem above at once.

**Copies are counted, deliberately.** A `duplicate_of IS NULL` filter belongs to
`RebuildInsightsCompanyGrowth`, which counts open *postings* for the leaderboard;
`RebuildInsightsCompanyResponse`, the rollup this design actually inherits, never joins `jobs`
at all and reads the denormalised `company_slug` off the event instead. Applying the posting filter here
would drop applications made against repost copies — the very population the cluster grain exists
to capture. It is also unnecessary: `RecomputeRoleDuplicatesForCompany` marks all but `min(id)`
*within* a fingerprint cluster, so grouping on the key already collapses canon and copies, and
`count(DISTINCT user_id)` dedups a candidate who applied to two copies of one role.

**The cluster key does not survive a prune, and that is a hole in the feature.** `cmd/prune`
clears `jobs.job_id` references and archives the row to `pruned_jobs`, which carries `source`,
`external_id`, `title`, `company_slug` and the matched rule — **but no `role_fingerprint`**
(migration `0041`). So a pruned posting's application can still be resolved to a *company* and to
an *application*, and never back to its *cluster*. With the pruning campaign slated to remove
roughly 1.5M of a 3.5M catalogue, that means a cluster's tracker count can shrink over time
because of an unrelated operator action — one more way a published number moves for reasons that
have nothing to do with the employer.

The remedy is the one the ledger already uses one field earlier: `application_events`
denormalises `company_slug` at write time *precisely because* prune clears `job_id`. This design
needs the same treatment for `role_fingerprint` — carried onto the application or the event when
it is written. Without it the aggregate is not durable, however well it is gated.

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

**Do not build a posting-level feature.** Aggregate at a `(company_slug, role_fingerprint)`
grain, reusing the existing gates and the `observable` definition.

`role_fingerprint` specifically, and never `internal/roletag`'s role slugs. Those are the more
visible notion — generated into the frontend as `ROLE_LABELS`, carried on the search document as
`Roles []string` behind the `roles` facet — and they are a per-posting **slice**, so clusters
built on them overlap and one candidate lands in several at once, inflating every gate this
design rests on. A fingerprint is one per posting and its clusters are disjoint.

**It cannot reuse `cmd/rollup-company`'s machinery, only its shape.** That worker is an atomic
`DELETE`-then-`INSERT` that recomputes every published value from current state on each run,
which is exactly the mutation channel the freeze rule above exists to close. The existing rollups
can rebuild safely because nothing they publish is attributable to an individual; this table
cannot. It has to be append-only at window close, with published rows never recomputed — a
different persistence discipline from every `insights_*` table in the tree today, and the main
reason this is not the small extension it first appears to be.

Everything else is deferred: stage-to-stage funnels (inputs untrusted), applicant counts on a
posting page (differencing), per-job consent (selection bias), and the free-text interview
reviews discussed in the issue's comment, which are a separate feature over
`internal/companyfeedback` and share none of this machinery.

## Do this first, before any implementation

One read-only measurement decides whether the rest is worth designing further:

> Over applications whose owner has a connected mailbox, what is the distribution of
> **distinct users per role cluster**, and how many clusters reach 10?

The `observable` CTE below is lifted verbatim from `RebuildInsightsCompanyResponse`
(`internal/db/queries/insights.sql`) — including the reason a calendar-only Google grant does not
count. It is the shipped definition, not the proposed one: *Freeze the metric inputs* argues that
the feature should stamp observability at **write** time, whereas this CTE evaluates it at read
time. That difference is deliberate here and it inflates in the same direction as the missing
opt-in predicate — a candidate who connected a mailbox only recently counts for every past
application in this query, and would count for none of them under the write-time stamp. It is one
more reason the histogram is a ceiling rather than a forecast.

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

**Read the result as an upper bound, and only in one direction.** Three biases apply, and they do
not cancel:

- **No opt-in predicate**, because the column does not exist yet. Under the account-level,
  default-off consent recommended above, the eligible population is a *subset* of the observable
  one. This inflates, and it dominates: a plausible opt-in rate divides the real figure several
  times over.
- **Pruned postings drop out**, since the join is on `application_events.job_id` and `cmd/prune`
  sets it to NULL. This deflates. How much is not something to assert — it is measured by the
  second query below.
- **Copies are included**, which is correct and not a bias at all (see *Copies are counted*).

Whether the histogram may be read as an upper bound therefore **depends on that second
measurement**, and the label should not be applied before it is run:

```sql
-- Upper bound on what the pruned-job blind spot costs the histogram above.
SELECT count(*) FILTER (WHERE ae.job_id IS NULL)  AS unclusterable,
       count(*)                                    AS observable_applied,
       round(100.0 * count(*) FILTER (WHERE ae.job_id IS NULL)
             / nullif(count(*), 0), 1)             AS pct_lost
  FROM application_events ae
 WHERE ae.kind = 'applied'
   AND ae.retracted_at IS NULL
   AND (EXISTS (SELECT 1 FROM gmail_connections gc
                 WHERE gc.user_id = ae.user_id AND gc.status = 'connected' AND gc.email <> '')
     OR EXISTS (SELECT 1 FROM mailboxes mb WHERE mb.user_id = ae.user_id));
```

Two properties make a small result conclusive. `job_id IS NULL` catches more than pruning — an
application recorded from mail against a company with no catalogue posting reads the same way —
so the figure **over**-states the loss, which is the conservative direction here. And it is an
absolute ceiling on the missing cluster-memberships: below the gate itself, it cannot create even
one qualifying cluster however it concentrates.

So:

- **`unclusterable` small against the first histogram's mass near the gate** → the upper-bound
  reading is earned. Inflation from the missing opt-in predicate dominates, and a disappointing
  result settles a **no-go**: if the ceiling does not clear the gate, the real figure cannot.
- **Otherwise** → the label is withdrawn, and the measurement is redone once `role_fingerprint`
  is persisted on the application and the pruned rows become clusterable.

In neither case does a healthy result license a *go*. That direction always requires re-measuring
against a consent column that does not exist yet. Getting the asymmetry backwards inverts the
decision rule, which is why it is stated rather than left to the reader.

One thing the bound cannot be aligned against: the recovery path it would be compared to — pairing
through `application_id` on a persisted fingerprint — **is a recommendation in this document, not
shipped**. A measurement cannot match a path that has not been built; what it can do is bound how
much that path would add, which is what the query above does.

If the answer is a handful of clusters concentrated at one employer, the feature does not exist
yet regardless of how it is designed, and the correct outcome is to close the issue as premature
and revisit when the tracker base has grown. That is a query, not a project, and it should be run
before anything else here is scheduled.

The query also cannot double as the feature's own aggregation. That would have to pair through
`application_id` the way the company rollup does, and cluster on a `role_fingerprint` carried on
the application rather than read from `jobs` — see *The cluster key does not survive a prune*.

## Open questions this pass did not settle

- **Bucket boundaries, hysteresis width, and the observation-window length** all need the
  distribution above to choose sensibly, and they trade against each other: a longer window
  raises N and immutability but describes a hiring round that has already closed. Picking any of
  them first would be a guess presented as a threshold.
- **The two sample gates need re-deriving at cluster grain.** 10 and 5 were judged for company
  grain, where no individual is visible; §3 argues 5 in particular is unsafe here.
- **How long the grace watermark runs**, and how much cohort the write-time observability stamp
  costs. The stamp trades a biased-but-large cohort for a smaller one whose bias is at least
  stable and nameable; whether that trade is worth making cannot be settled without knowing how
  many candidates connect a mailbox only after they have started applying.
- **Repair versus immutability.** Under published snapshots a mislinked reply inside a closed
  window cannot be retracted out of it. Whether that is answered with an operator republish, a
  correction that only affects future windows, or a shorter window is a product decision this
  pass deliberately leaves open.
- **Round-aware stages** (the issue's comment) would sharpen stage timing, but only for
  transitions that mail or calendar evidence can date. It does not change the trust rule, so it
  is a smaller unlock than it looks.

## Related

- `openspec/specs/company-hiring-signal/spec.md` — the shipped company-level aggregate and its gates.
  **Stale on one point:** it states the median is served "under the same ten-application sample
  gate as the response rate it accompanies", which the code has not done since `replySampleGate`
  landed (`internal/handler/company_response.go`). The code is right and the spec is wrong; worth
  correcting separately, and it is what misled the first draft of this document.
- `internal/handler/company_response.go` — `responseSampleGate` / `replySampleGate`, the two gates
  over two denominators
- `openspec/specs/application-event-ledger/spec.md` — what the ledger guarantees and what it refuses to backfill
- `internal/appevent/appevent.go` — `TrustedForDayMath`, the trust rule
- `internal/userjob/AGENTS.md` — stages, the silence ladder, why the ledger and not the columns feed aggregates
- `internal/ghost/AGENTS.md` — `ContributorGate = 2`, the existing per-posting anonymity gate
- `internal/companyfeedback/companyfeedback.go` — rate limiting and report/hide for user-contributed content
