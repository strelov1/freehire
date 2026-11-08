## Context

What exists, and what it settles:

- **The ledger** (`application_events`, `openspec/specs/application-event-ledger`) is append-only and
  content-free: `applied`, `employer_reply` (with the classifier's signal), `follow_up_sent`,
  `stage_set`, `interview_scheduled`, each with `occurred_at` and a `source`. Events are paired to
  their application through `application_id`, which survives `cmd/prune`; `job_id` does not.
- **Only some sources are evidence.** `appevent.TrustedForDayMath` admits mail and calendar
  sources and refuses a stage the candidate typed, because that dates the candidate's diligence,
  not the employer's behaviour. Stage history is deliberately not backfilled, so stage-to-stage
  timing has no data older than the day `stage_set` started being written, and even then only
  where mail or a calendar dated it.
- **A per-company aggregate already exists** (`ComputeInsightsCompanyResponse`, served by
  `companyResponseRate`): observable applications (a connected Gmail or hosted mailbox), answered
  ones, a median reply time, gated at 10 and 5 (`userjob.ObservableSampleGate`, `replySampleGate`).
  It carries no consent switch and no per-application opt-out.
- **Anonymity precedent for a per-person surface** is interview buddies: opt-in membership, a
  feed vocabulary that excludes `accepted`/`rejected`/`withdrawn`/`expired` ("took the offer is the
  most identifying fact in the ledger"), day precision, a per-application hide keyed on
  `application_id`, and `jobs.is_private` postings never shown.
- **Repost identity** is the duplicate closure (`openspec/specs/job-cluster-copies`), deliberately
  not `role_fingerprint`, so that search geography and the copies list never disagree.
- **Layering.** `application` (layer 6) may read `job` (5) but `job` cannot read `application`.
  The public job wire shape (`jobview`, layer 5) therefore cannot carry a figure computed from
  tracking data.

The task text refers to `hire-tracking-calendar.md`, `hire-application-event-ledger.md` and
`hire-repost-marker-lag-window.md`; none is in this repository. This design used the specs and
AGENTS.md files above, which cover the same ground, and found nothing named "repost marker lag
window" beyond the duplicate-marker passes, which is what the cluster decision relies on.

## Goals / Non-Goals

**Goals:** a per-role, opt-in, anonymized view of reply rate, time to first reply and outcome
split; a publication rule a reviewer can check by reading one function; a way to find out cheaply
whether any of it will ever render.

**Non-Goals:** stage-to-stage timing (no data, see below); salary or compensation; any figure about
an individual; changing the existing company rate; changing `applied_count`; showing raw counts
anywhere; employer-facing analytics.

## Decisions

### 1. The unit is the role cluster, windowed to 180 days

A single posting row is the wrong unit twice over. It is too small: the same role is posted per
city, per board and again after expiry, and each copy would start at zero. And it is the unit
`user_jobs` happens to key on, not the thing a reader means by "applying for this job".

The aggregate is keyed on the cluster's ultimate owner and counts every application whose posting
is in that owner's closure, resolved by the same function `GET /jobs/:slug/copies` uses. It
must not be rebuilt from `role_fingerprint`: the copies spec explains why two answers to "which
postings are the same role" would put a user on a page that disagrees with itself.

Only applications dated within the last 180 days count (`applications.applied_at`, which the
ledger's `applied` event mirrors after a re-date). Hiring processes drift with headcount, tooling
and the recruiter, and a figure that averages two years reads as the role's character when it is
the previous team's. 180 is a judgement, not a measurement; it is a constant beside the floors so
that it can be tuned against the report in task 0.

When a repost is not clustered by the dedup passes, it starts a new cluster at zero and the figure
is absent. That is the conservative failure: absent, never wrong.

Staleness needs no separate mechanism. The aggregate is recomputed from scratch daily, so a
cluster that gains or loses members, or whose owner pointer moves in a later dedup pass, is
correct within a day. Nothing about a cluster is stored incrementally.

### 2. One vote per member per cluster

The count is DISTINCT members, taking each member's earliest application in the cluster. A member
who applied to the Berlin and the Munich copy is one contributor, and is one observation of "how
long until they heard back", not two. Counting applications would let one active member reach the
floor alone and would hand one person's experience double weight in every share.

### 3. Population: observable, mature, evidence-sourced

Same definition of observable as the company rate and for the same reason: a member with no
connected mailbox produces no `employer_reply` events, and counting them would report our blind
spot as the employer's silence. Three rules are added for a figure about a role rather than a
company, each from a hazard rather than a preference:

- **Mature only.** An application counts only once it is older than the `applied` silence
  threshold in `internal/job/silence` (21 days, measured over 92 observed applications). Without
  this, every application made this week is "no reply", and a role that just started getting
  traffic reads as hostile. Imported, not restated: tuning one number must tune both.
- **Evidence only.** Outcomes come from `employer_reply` events whose source is `mail_gmail` or
  `mail_hosted`, and from calendar interviews. `mail_external` is pushed by the caller's own
  harness through an API key, so anyone with an account can write any reply they like; it is fine
  for a member's own board and not for a public figure. A stage the member typed never counts.
- **Not private, not hidden.** `jobs.is_private` postings and applications the member switched
  off are excluded from both sides of every ratio.

The company rate shares none of the three. See "Existing company rate" below.

### 4. What is published

One object per cluster, or nothing:

| field | rule |
|---|---|
| `contributors` | a band: `10-24`, `25-49`, `50+` — never the number |
| `heard_back_pct` | share of contributors with any employer reply, nearest 10% |
| `outcomes` | `no_reply`, `rejected`, `advanced` as shares, nearest 10%, summing to 100 after rounding |
| `reply_days` | `{low, high}`: the 25th and 75th percentile of days to first reply, rounded to whole days, only if at least 5 contributors were answered |
| `window_days` | 180 |

`advanced` folds screening, assessment and interview invitations and offers into one class. An
offer, and above all an acceptance, are the facts that identify a person, and buddies already drew
that line; this surface draws it at least as tightly. Interview counts from calendar events are
folded into `advanced` too, never a separate "interviewed" share.

A range for the reply time rather than a median: it says the spread, which is the useful part
("usually 3 to 9 days" tells you when to chase), and it is the quantity that does not move by a
visible step when one member is added. The median is right-censored (answered applications only);
`heard_back_pct` beside it keeps the page from reading the reassuring half of a two-part fact, the
same reason `companyResponse` carries `Unanswered`.

The rejection share is a lower bound and the copy says so: many employers never send a rejection,
and those applications read as `no_reply`.

Not published, in any form: exact counts, per-stage counts, anything per day, the share of
contributors who used auto-apply, a distribution of offer outcomes.

### 5. The floor and the two attacks it does not stop

Nothing is published below 10 distinct contributors, the same number the company rate uses
(`userjob.ObservableSampleGate`, imported). The reply range has its own floor of 5 answered, not
shared, for the reason `replySampleGate` gives: the two figures have different denominators.

Cluster rows below the floor are NOT written to the rollup table. A table holding `contributors =
3` and a gate in the handler is one missing `if` away from publishing it, and from the outside it
makes "no row" mean "zero". With no row, absent means only "not enough to say".

The floor is a defence against reading one person out of a small group. Two attacks survive it,
and the design takes a position on each rather than leaving them unstated:

- **Differencing.** Someone who watches a page daily, and knows who applied, sees the figure move
  when a member joins or an outcome changes. Mitigation: quantization at write time (a 10% band
  needs a swing to cross), a daily snapshot rather than live reads, and a floor of 10. Residual
  risk is accepted: it needs an attacker who already knows nine of the ten members' outcomes, at
  which point the figure reveals one fact about a person the attacker has a reason to be looking at.
- **Intersection with `applied_count`.** The existing exact `applied_count` plus a published
  `contributors` band lets a reader infer how many tracked members did not opt in. That reveals
  nothing about any of them. Accepted.

### 6. Opt-in, account level, with a per-application hide

Default off. The decision the task asked for is between a per-job consent and an account setting;
the account setting wins, with a per-application exception:

- A per-job prompt lands at the moment the person is busiest and least interested in an abstract
  data question. The most likely answer to a prompt that is in the way is whatever dismisses it.
- Applications differ in sensitivity, and the account setting alone cannot express "not the one at
  my current employer's competitor". The per-application hide does, by the mechanism buddies already
  proved (`buddy_hidden_applications`), as a nullable column on `applications` rather than a table:
  `applications` rows already cascade with the member, and a nullable column with no default does
  not rewrite the table.
- The consent is a timestamp (`users.share_process_signal_at`), not a boolean: it records when it
  was given, and NULL is off, so a revoke is a write that cannot leave a stale `true`.
- Consent is cookie-only. A scoped API key, or the assistant acting for a member, must not be able
  to enrol a person in sharing.

Revocation and account deletion need no cleanup path because the aggregate stores nothing per
member. Each rollup deletes and recomputes, so a member who revokes, hides an application, or
deletes their account disappears from every figure at the next run (within a day), and the page
copy says "within a day" rather than "immediately".

**The cost is real and the report in task 0 exists to size it.** Opt-in rates for a switch the
member has no urgent reason to flip are low. If a fifth of observable members opt in, a cluster
needs ~50 observable applicants to show ten. The honest expectation is that very few clusters
clear at launch.

**Where it is asked.** In the tracker (`/my/tracking`), once, after the member's first application
has matured and had an outcome, framed as a trade ("see how long people wait to hear back at roles
like this; add yours anonymously"), and in account settings permanently. Not a modal at apply time.

**Reciprocity (view only if you contribute) is not proposed.** The figure is public like the
company rate; gating it behind consent would make the page unreadable to crawlers and the people who
have not signed up yet, who are the audience for a feature that markets the product. Reconsider if
the figures exist and contribution stalls.

### 7. Existing company rate

The per-company rate collects from every observable member without consent, includes
`mail_external` events, and counts immature applications, so it under-reports replies for a
company that is currently receiving traffic. It is shipped, public, and the right tool at company
grain, and this change does not touch it. It is recorded here because opt-in for the new figure
beside opt-out for the old one is an inconsistency a member can find, and the privacy policy
currently says only that interactions are stored "so we can show you your pipeline".

**Recommendation:** leave it for this change; open a follow-up to give it the same three
rules (consent, evidence-only, maturity) once the new aggregation exists to share code with. The
alternative, moving it under the consent now, would empty a published figure to make a product
decision that has not been asked. The owner should confirm.

### 8. Where the number lives, and what serves it

- **Rollup, not a request-time query.** The closure resolution plus a ledger join per page view
  would be expensive on the busiest page, and a live figure is what makes differencing easy.
  `insights_process_signal` is rebuilt by one more pass in `cmd/rollup-company`, using its
  compute-on-the-read-connection, publish-on-the-writer pattern. No new host unit.
- **A separate endpoint**, `GET /jobs/:slug/process-signal`, not a field of the job. `jobview`
  cannot import `application`, the list and search paths would pay for it, and a null result on 99%
  of postings should cost one cached request, not a wider payload everywhere. The endpoint is
  additive, so the oasdiff gate stays quiet.
- **A new package, `internal/application/processsignal`**, for the rules (quantizers, floors,
  windows) as pure functions with table tests, so the publication rule is one place a reviewer can
  read. It goes in the layering table in the same change.
- **The handler returns what the rollup stored and nothing else.** A test asserts the body holds no
  raw count; the raw count is not in the table to leak.

### 9. Where it does not apply

The page shows nothing when: the cluster is below the floor; the posting is private; the posting
is an on-site employer vacancy (`internal/ingest/directapply`: we are the ATS there, the outcome is
ours to know, and the employer could shape it); or a moderator has suppressed the
cluster after a dispute (a new, small table the compute query reads).

**Stage-to-stage timing (applied to screen to interview to offer) is out of scope** and is the
largest of the issue's four asks. The ledger has no stage history before the day `stage_set` was
written, a typed stage is not trusted for day math, and what remains is "a mail was classified as a
screen on this date", on a population already under the floor. Revisit when the report shows a
cluster count where `interview_scheduled` coverage is not a handful.

Sparse logging is handled by evidence-only counting: a member who never connected a mailbox, or who
logged every stage by hand, contributes nothing rather than a guess. Members who log inconsistently
cost sample size, never accuracy.

### 10. Abuse and gaming

Realistic attacks and what each costs:

- **An employer inflates reply rate** by opening accounts, connecting mailboxes and replying to
  themselves. It needs a Gmail or hosted mailbox per fake member, messages the classifier
  recognizes as ATS mail, ten distinct members per cluster, and an opt-in each. Possible, not free.
  The rules that make it dearer: one vote per member, `mail_external` excluded, mature
  applications only, consent required.
- **An employer depresses a competitor's rate** the same way, with no replies. Same cost, same
  rules, and `no_reply` needs maturity, so it cannot be done in a week.
- **A mass applier** (including auto-apply) drags reply rate down for the roles it sprays. One
  vote per member limits it to one voice per cluster. Auto-apply submissions are COUNTED in this
  change: they are real applications to the role, and reply rate is reasonably "what happens when
  you apply here". `applied` events carry `source = auto_apply`, so a split is measurable later and
  nothing is lost by deciding later. The owner may exclude them instead.
- **Selection bias** is not an attack and cannot be fixed: members who connect a mailbox and opt in
  are not all applicants. The copy says "members", never "applicants".

None of this makes the signal unforgeable, and the design does not claim so. Two operational
backstops are cheap: the rollup logs per-cluster contributor counts so a cluster reaching the floor
from many brand-new accounts in one day is visible, and a moderator suppression list (a table
read by the compute query) lets a disputed cluster be pulled without a deploy. A published
statement about a named employer will draw disputes; the existing company page already has that
exposure.

## Risks / Trade-offs

- **It may never render.** The most likely outcome of a correct implementation is a card that
  appears on a small share of postings. That is why task 0 comes first, and why the cheapest useful
  step if the report disappoints is task 5, not nothing.
- **Opt-in lowers an already small sample.** Accepted as the price of consent; opt-out was
  considered and set aside because the issue asks for opt-in and because an aggregate published
  about named roles is a stronger claim on a member than a figure only they see.
- **180 days, 21 days, bands and the 5% go-line are judgements.** Each is a named constant or a
  stated threshold, so the first report can move them without redesign.
- **The privacy policy lags the existing company rate.** Out of scope here and flagged in
  decision 7.

## Open Questions

1. Should the existing company rate move under the same consent, evidence and maturity rules?
   *Recommendation: not in this change; follow-up.*
2. Show the existing company rate on the job page now, ahead of everything else? It costs one
   handler call, adds no new data, and gives a job page something while the role figure is absent.
   *Recommendation: yes, as task 5, but only after question 1 is answered, so the page does not
   advertise a figure the owner is about to re-scope.*
3. Do auto-apply submissions count? *Recommendation: yes, revisit with the split.*
4. Is 5% of job-page views at 20% opt-in the right go/no-go? *Recommendation: yes; it is a
   judgement and the owner owns it.*
