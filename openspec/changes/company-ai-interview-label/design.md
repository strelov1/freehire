## Context

Two mechanics already in the codebase decide most of this design, and copying
them is cheaper than inventing a third of anything.

**The report dialog already splits on evidence.** `ReportDialog.svelte` routes
four reasons to the moderation queue and one — `no_response` — to the ghost
signal, and `$lib/reports.ts` pins the split in `isEvidenceReason` with a comment
explaining why: the moderation queue's only lever is closing the job, so a report
describing what happened to a person reaches a reviewer who can do nothing useful
with it. `ai_interview` is the same shape. There is nothing for a moderator to do
about a true statement.

**A company-level facet already reaches the jobs index.** `companies.collections`
is written by `cmd/import-collections`, denormalized onto `jobview.Job.Collections`
("denormalized from the company onto the job", jobview.go:68), and declared
filterable on the jobs index (client.go:600). That is the whole path from a
company attribute to a search filter, already load-bearing for `yc` and `bigtech`.

What does **not** fit is `company_feedback` (migration 0088). It is a 1–5 star
review with free text and a closed `feedback_type` that already includes
`'interview'` — but it is one row per user per company, and it demands a rating.
A star forces a judgement where this needs a fact, and the uniqueness bound means
a user who already reviewed the compensation could never report the interviewer.

The catalogue currently carries 203 open micro1 postings and the postings of most
of the companies in this category.

## Goals / Non-Goals

**Goals:**

- A candidate can see, before applying, that an employer screens with an AI
  interviewer — on the job card, the job page and the company page.
- A candidate who does not want that can filter those employers out of search.
- Filing the fact takes one interaction from the page where they met it.
- The signal self-heals: a report can be withdrawn without deleting the row.

**Non-Goals:**

- **No judgement.** The label states a practice; it does not rank, warn or score.
  No red, no shield icon, no "beware".
- **No moderation queue.** Nothing here produces work for a reviewer.
- **No detection.** We do not infer the label from a description, an ATS or a
  vendor's script tag. That is a separate, harder question; this change is only
  the candidate-reported path.
- **No per-posting precision.** A large employer running a bot on some roles and
  not others will carry the label company-wide. That is the accepted cost of
  reaching useful coverage at all (see Risks).
- **No new report surface.** The reason joins the dialog that already exists.

## Decisions

### The evidence attaches to the company, not the posting

**Chosen:** one row per `(user, company, kind)`; the report is filed from a job
page and the company is resolved from the job.

**Alternative rejected:** per-posting, mirroring `ghost_reports` exactly. It is
less new code and it is where the reporter actually was — but micro1 alone has
203 open postings, each needing its own reporter before its own card shows
anything. A signal that can only ever describe the postings somebody personally
sat an interview for never reaches the candidate it is for. An AI interviewer is
a property of the employer's process; storing it against one posting is storing it
in the wrong place and paying for the mistake in coverage.

### A new table rather than `company_feedback`

**Chosen:** `company_process_reports`, modelled on `ghost_reports`.

```
id            bigint  generated always as identity primary key
user_id       bigint  references users (id) on delete cascade
company_slug  text    references companies (slug) on delete cascade
kind          text    not null, CHECK in a controlled list
created_at    timestamptz not null default now()
retracted_at  timestamptz
UNIQUE (user_id, company_slug, kind)
```

`user_id` is `NOT NULL` and cascades, unlike `company_feedback`'s
`ON DELETE SET NULL`: feedback is authored content that outlives its author
de-authored, while this is a countable claim that means nothing without a claimant.

**Retraction, not deletion**, for the reason `ghost_reports` gives: withdrawing is
how a signal self-heals when the employer changes practice, and keeping the row
preserves the uniqueness bound so a retraction cannot be used to file repeatedly.

### `kind` is a column with a CHECK list, holding one value today

**Chosen:** `kind text NOT NULL CHECK (kind = ANY (ARRAY['ai_interview']))`.

**Alternative rejected:** a table dedicated to the one signal, no `kind`.

This is the judgement call in the change, so the reasoning is on the record.
The family is real and named: an AI interviewer, an unpaid test task, government
ID demanded before an offer — facts about a hiring process a candidate discovers
too late, each with identical storage, identical uniqueness, identical retraction
and identical counting. Three tables that differ only in their name would be worse
than one column, and the CHECK list is what keeps the column from becoming a free-
text dumping ground. Adding a second value later is one migration line and one
constant; it is deliberately **not** a config knob, because the vocabulary decides
what the UI must render and the facet must declare.

The single value shipping today is not a placeholder — no other kind is built,
surfaced or filterable in this change.

### One report is enough to show the label

**Chosen:** the label appears at `count >= 1`, and the count ships beside it.

`no_response` needs a contributor gate (`ghost.ContributorGate`) because one
person's silence may be bad luck, an inference from an absence. "A bot conducted
my interview" is an observation, and a single observer knows it as well as forty.
Gating it would mean the first four honest reporters see nothing change and stop
reporting.

The count is shown for exactly the reason the threshold is one: a reader deserves
to weigh `1 report` differently from `40 reports`, and a bare badge hides that
difference. The counter is materialised on `companies` and recomputed in the
writing transaction, the pattern `feedback_count` and `upvote_count` already use,
so a reader never sees a label without its number.

### The counter is a `jobs` column, synced inside the report transaction

**Chosen:** mirror `collections` — a denormalized column on `jobs` — but write it
in the same transaction as the report, rather than on a worker's schedule.

`jobs.collections` is a column that `cmd/import-collections` syncs from
`companies` after it writes them (jobs.sql:1077-1086), bumping `updated_at` so
`reindex --since` picks the rows up. Two things about that pattern are separable,
and it took getting them backwards once to see it:

- **The column is right.** `jobview.FromRow` takes a bare `db.Job`, and
  `job.Extras` — documented as holding exactly this, "denormalized from the
  company" — is built from that row. A column means sqlc regenerates, `Extras`
  gains a field, the wire shape gains a field, the search document gains it by
  embedding, and **no query signature and no caller changes**.
- **The worker's schedule is wrong.** Membership in a collection changes rarely,
  so hours of staleness are invisible. A report is filed in real time: the company
  page would show the label the moment somebody filed while that same company's
  job cards said nothing until the next sync. The same fact present on one page
  and absent on the next reads as a bug, and no wording fixes it.

So the sync runs in the write: one statement beside the company recompute that writes
the column AND queues the changed open postings into `search_outbox`. It is one
statement, not a loop, and filing a report is a rare action — the largest employers here
carry thousands of open postings and a few thousand row updates in one statement is
unremarkable.

**The enqueue was missing in the first version, and the gap is worth recording.**
`PropagateCollectionsToJobs` only bumps `updated_at`, and its comment points at
`reindex --since` — a mode `cmd/reindex` does not have. Copying that shape meant nothing
carried the change into Meilisearch: on the first real report the badge appeared on the
job page (served from Postgres) and on no card, and the filter matched nobody. The rows
had to be pushed into `search_outbox` by hand to make the feature visible at all. Only
OPEN postings are queued — a closed one is not in the index, so a row for it is work the
drain would do and discard — while the column is written for closed rows too, so a
posting that reopens already carries the right count.

**Alternative rejected: JOIN `companies` on the read.** No job read query joins
companies today, so adding one changes `ListJobs`/`GetJobBySlug` from returning
`db.Job` to returning generated row types, which ripples through every caller of
the projection. That is a wide, invasive change to carry one integer, and it buys
nothing the column does not already give.

**Alternative rejected:** filter on the companies index and intersect. The jobs
index is what `/jobs` queries; a second round-trip to resolve company slugs would
add a query to the hot path to avoid one field on a document.

The wire shape carries the count, not just a boolean, because the badge needs the
number and the job detail response is where it comes from.

### The report is filed against the company, and the URL says so

**Chosen:** `POST /api/v1/companies/:slug/process-reports` with
`{ "kind": "ai_interview" }`; `DELETE` on the same resource retracts.

The dialog opens from a job, and the job view already carries `company_slug`, so
the client resolves it without an extra call. Routing it through
`POST /api/v1/jobs/:slug/reports` would make an endpoint documented as storing a
`pending` moderation report against a job do neither of those things — the exact
confusion the `no_response` split already had to work around with a second
endpoint (`reportGhostJob`). An honest URL costs one route.

Auth, rate limiting and the 409-on-duplicate answer follow the existing report
endpoints; a caller who already filed gets 409, not a silent second row.

## Risks / Trade-offs

**A filterable attribute rolled out before its index settings → every request for
that filter is a Meili 400, which the search package maps to a 500 for every
caller of the filter, not only the one who ticked the new box.** → The settings
patch lands on the live index **before** the binary that queries it. The
`search-settings-drift` worker publishes the gap as a gauge; it makes the mistake
visible, it does not prevent it. Ordering is a human decision and belongs in the
task list, which is where it is.

**Incremental `search_outbox` pushes will not carry the new field** — they only
send documents whose `content_hash` moved, and this is not part of that hash. →
A **full reindex** after the first labels land, exactly as `is_tech` and
`requires_clearance` both needed after their backfills. Until it runs the filter
matches fewer employers, never wrong ones.

**Deploying the binary before the migration → 42703 on every company read.** →
Migration applied to prod first. `initdb` runs migrations only on first volume
init, so on the persistent prod volume this does not auto-apply — the same manual
step 0005–0010, 0039–0041 and 0088 all carry.

**A company-wide label is wrong for a large employer that runs a bot on some
roles only.** → Accepted, and made legible rather than hidden: the count is
always shown, retraction is available, and the label states a reported practice
rather than a policy. Per-posting precision was considered and rejected above for
coverage; if this becomes a real complaint the counter is the evidence for
revisiting it.

**One user could label many companies.** → The uniqueness bound stops repeat
filing per company, the endpoint carries the same daily rate limit the report
endpoints already answer 429 on, and the count exposes a lone reporter as a lone
reporter.

**A retracted report leaves the count stale if the counter is recomputed
elsewhere.** → The counter is recomputed in the same transaction as both the write
and the retraction, in Go, never by a background job. There is one writer.

## Migration Plan

1. **The migration needs no hand-running.** `deploy/bin/release.sh` builds and runs
   `cmd/migrate` itself — idempotent, one transaction per file, recorded in
   `schema_migrations` under an advisory lock — and it runs BEFORE the new colour
   starts, so that colour never serves a request against an older schema. A failing
   migration aborts the release with the live colour untouched. (The "APPLY TO PROD
   MANUALLY BEFORE DEPLOY" header these migrations carry describes the Docker `initdb`
   path, which is a different one; taking it at face value is what made an earlier
   revision of this plan claim a manual step that does not exist.)
2. **Meilisearch settings patch to the live jobs index** — the new filterable
   attribute, before the release. This one IS a hand step, and `release.sh` guards it:
   after the health check it runs a facet smoke against `/api/v1/jobs/facets` and
   refuses to flip when a declared attribute is not live, so a missed patch costs a
   failed release rather than an outage.

   **It must not be applied while `freehire-reindexw` is running.** A rebuild streams
   into a throwaway index carrying the DEPLOYED binary's settings and swaps it over the
   live one, so a patch applied mid-rebuild is silently discarded by the swap.
3. **Deploy the binary and the frontend.** The label is `0` everywhere; the badge
   renders nowhere; the filter matches nothing. This is a correct empty state, not
   a broken one.
4. **Reports accumulate.** No backfill exists or is possible — nobody has reported
   anything yet, and the change deliberately does not infer the label.
5. **Full reindex** once the first labels land, so the facet sees pre-existing
   documents.

**Rollback:** the frontend hides the badge and the filter (the reason leaves the
picker), which makes the whole feature inert without a database change. The table
and column are additive and can stay. Nothing else reads them.

## Open Questions

None blocking. Two noted for after the first reports land:

- Whether the badge on the **job card** (as opposed to the job page) is worth its
  visual weight at low counts. Shipping it on the card is the decision; the count
  is what tells us if it reads as noise.
- Whether `unpaid_test_task` and `id_before_offer` are worth adding. The `kind`
  column makes that a small change; nothing about it is decided here.
