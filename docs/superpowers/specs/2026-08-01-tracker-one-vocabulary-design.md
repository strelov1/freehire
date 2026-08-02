# One state, one name: the tracker's vocabulary

## Problem

An application has one state, stored in one column (`applications.stage`), and the tracker
shows it under three different names at once:

| stage | board column | pipeline bucket |
|---|---|---|
| `applied` | Applied | `no_answer` |
| `screening` | Applied | `in_progress` |
| `responded` | Applied | `in_progress` |
| `interview` | Interview | `interviewing` |
| `offer` | Offer | `offer` |
| `accepted` | Closed | `accepted` |
| `rejected` | **Closed** | `rejected` |
| `withdrawn` | Closed | `declined` |

A rejected application therefore reads as `Rejected` in the drawer's selector, sits in a
column labelled `Closed`, and is counted in a bucket called `rejected` — while two of the
seven bucket names (`in_progress`, `declined`) appear nowhere else in the product. None of
the three vocabularies is a subset of another, and each is defined in its own place:

- `internal/userjob/stages.go` — the 8 stages (the stored truth)
- `web/src/lib/board.ts` — `STAGE_COLUMN` + `BOARD_COLUMNS`, the 4 columns
- `internal/userjob/buckets.go` + `web/src/lib/pipeline.ts` — the 7 buckets
- `web/src/lib/components/HomeFunnel.svelte` — a **fourth** copy of the bucket vocabulary
- `web/src/lib/stages.ts` — `STAGE_LABELS`, the labels

A fourth vocabulary — the mail classifier's 9 status signals — maps into stages through a
private table in `internal/mailclassify`, and the mapping is invisible in the UI. Mail that
plainly announces a rejection moves nothing, by design, and says so nowhere: the candidate
sets the stage by hand and cannot tell why.

## Goals

1. One stage vocabulary with one label per value, read by every surface.
2. Group membership (the board's columns, the funnel's bands) defined once, in Go.
3. The mail signal → stage relationship visible where the mail is read, and a one-click
   resolution when a message and the stage disagree.

## Non-goals

- Changing the stage vocabulary itself. The 8 values stay; the data is not migrated.
- Changing when mail advances a stage. `mailclassify.AdvanceStage` and its thresholds are
  untouched: a rejection still never advances automatically.
- The drawer's `Viewed → Saved → Applied` strip. It renders an engagement funnel in a shape
  that reads as a timeline (newest on the left), which is a real defect — but the honest
  chronology already exists on the Calendar tab, and rebuilding the strip is its own change.
  Recorded here as known debt.

## Design

### 1. `internal/userjob` gains the group table

The package already holds three tables keyed on the stage vocabulary: `activeRank` and
`terminalStages` (`pipeline.go`), and `silenceThresholds` (`silence.go`). Groups join them:

```go
// Group is the coarse state the board shows as a column and the funnel as a band.
// Stages are the fine state inside it.
type Group struct {
	ID     string   // "applied" | "interview" | "offer" | "closed"
	Label  string   // "Applied"
	Stages []string // in pipeline order
}

// In pipeline order. Membership is exactly the mapping the board already applies:
//   applied   → applied, screening, responded
//   interview → interview
//   offer     → offer
//   closed    → accepted, rejected, withdrawn
var Groups = []Group{ /* ... */ }

func GroupOf(stage string) string // "" for an unknown stage
func Label(stage string) string   // "Rejected"
```

`TestEveryStageBelongsToOneGroup` binds `Groups` to `Stages` in both directions, in the shape
`TestEveryStageIsRankedOrTerminal` already uses: a stage added to the vocabulary and missed
here fails the build rather than rendering as a blank column.

`buckets.go` is **deleted** — `BucketCounts`, `Pipeline`, and `Aggregate`. It is the third
vocabulary, and the point of this change is that it stops existing.

Labels live in Go rather than staying in `stages.ts` because the vocabulary has a second
reader that is not a browser: the in-app assistant calls `internal/jobtracking` directly with
the session owner's id and never passes through Fiber. This is the rule `internal/inbox`
states for mail — a rule enforced in a handler is a rule the in-process reader never meets.

### 2. The contract carries stages, not buckets

`GET /me/tracking/pipeline` returns per-stage counts:

```json
{"data": {"applications": 49,
          "stages": {"applied": 12, "screening": 3, "responded": 0, "interview": 6,
                     "offer": 0, "accepted": 0, "rejected": 28, "withdrawn": 0}}}
```

The `buckets` object is removed. A repo-wide grep found no consumer outside this repository —
not the CLI, MCP skills, or the ChatGPT Actions surface — so this is a breaking change with no
known caller. Grouping is not returned: it is static, and returning it would put the mapping
in two places again.

`cmd/gen-contracts` emits `STAGE_LABELS` and `STAGE_GROUPS` beside the existing `STAGE_VALUES`.
Every frontend copy of the vocabulary becomes derived:

| file | before | after |
|---|---|---|
| `web/src/lib/stages.ts` | own `STAGE_LABELS` | reads the generated labels |
| `web/src/lib/board.ts` | own `STAGE_COLUMN`, `BOARD_COLUMNS` | reads `STAGE_GROUPS` |
| `web/src/lib/pipeline.ts` | own `PIPELINE_BUCKETS` | reads `STAGE_GROUPS` |
| `HomeFunnel.svelte` | own `VOCAB` | reads `STAGE_GROUPS` |

`interviewRate` and `offerRate` are computed from stages (`interview + offer + accepted` over
`applications`) rather than from buckets. Same arithmetic, one fewer indirection.

Also updated: `docs/API.md`, `web/src/lib/docs/api-spec.ts`,
`internal/handler/me_pipeline_integration_test.go`, `internal/jobtracking/repository.go`, and a
MODIFIED delta against `openspec/specs/application-pipeline/spec.md`, which currently pins the
seven bucket keys as a SHALL.

### 3. UI

- **Board** builds its columns from `STAGE_GROUPS`. The per-card stage badge already exists
  (`BoardCard.svelte:84`) and is what visually ties the `Closed` column to a `Rejected` card.
  The outcome dialog on a drop into Closed stays: the 4→8 reverse mapping is genuinely
  ambiguous, and asking is the honest resolution.
- **Funnel** renders four bands with a per-stage breakdown under each.
- **Drawer** groups the stage selector with `<optgroup>` by the same four groups, so `Closed`
  becomes a heading over Accepted/Rejected/Withdrawn instead of a competing concept.

### 4. Mail

`mailclassify` exports `StageFor(sig StatusSignal) (stage string, advances bool)` in place of
the private `signalStage` table, and the mapping is emitted into the contracts beside the
existing `EMAIL_STATUS_SIGNAL_VALUES`. Each message in the drawer's Emails tab renders its
signal and what it implies: `Acknowledgement → Applied`, or `Rejection → does not move the
stage`.

**Divergence suggestion.** `internal/jobtracking` computes a `stage_suggestion` on
`TrackedApplication` when the newest classified linked email implies a stage that differs from
the current one:

```json
"stage_suggestion": {"stage": "rejected", "signal": "rejection", "email_id": 8814}
```

The drawer offers to apply it in one click. It is computed in the service and not in the
handler for the reason above — the assistant is a second reader.

**There is no confidence threshold, and that is a schema fact rather than a preference.**
`emails.match_confidence` is the *matcher's* confidence in the link, not the classifier's
confidence in the signal — `migrations/0020` says so in its column comment, and
`mail_classification.sql` keeps it pinned to the link for that reason. The classification
confidence exists only in memory while `cmd/classify-mail` runs and is never persisted, so a
`>= 0.8` rule would need a new column written by the classifier.

That column is not worth adding yet. The signal is already trusted enough to be rendered as a
label on the message, and every suggestion is confirmed by a human before it changes anything.
If suggestions turn out to be noisy in practice, persisting the classifier's confidence is a
small, separable change — with the measurement that justifies it.

**How the suggestion goes away** is the part worth getting right: there is no `dismissed`
column. If `application_events` holds a `stage_set` newer than that email, the candidate has
already answered the question and the suggestion is not offered. The ledger exists to record
exactly this, and a new piece of state would be a second thing to keep true. A new query,
`LastStageSetAt(user_id, job_id)`, reads it.

This does not weaken the rule that mail never advances into a terminal stage. It surfaces the
rule: `userjob/pipeline.go` says deciding an application is rejected is the candidate's call
and never an inference from a message. The suggestion leaves the call with the candidate.

## Testing

- `TestEveryStageBelongsToOneGroup` — every stage in exactly one group, both directions.
- The existing `TestEveryStageIsRankedOrTerminal` and `TestSilenceThresholdsGrowStricter`
  keep guarding the other two tables; all three are now verified against one vocabulary.
- `me_pipeline_integration_test.go` asserts the `stages` shape and that the counts sum to
  `applications`.
- A frontend type-level check that `STAGE_GROUPS` covers `STAGE_VALUES`, in the
  `satisfies Record<K, V>` form the architecture review found stronger than a runtime test
  (it runs in the required `pnpm run check` gate).
- Suggestion rules, as service tests: a rejection email against an `interview` stage offers
  `rejected`; the same email offers nothing once a later `stage_set` exists; an email whose
  signal already matches the current stage offers nothing; an unclassified email
  (`status_signal IS NULL`, which every `external` message is by design) offers nothing.
- Each guard is verified by mutation — break the code, confirm the test names the culprit.

## Part B — tracker load performance

Split out deliberately: it shares no code with Part A and is gated on a measurement that has
not been taken yet.

### Diagnosis (from reading the code; not yet measured)

`/my/tracking` server-loads 500 rows in one call (`web/src/lib/server/tracking.ts:11`,
`listMyJobs('board', 500, 0)`). In descending order of suspicion:

1. **Every row carries the full job description.** `myJobResponse.Job` is a `*jobview.Job`,
   which includes `Description string` — the complete posting text. The board renders none of
   it. At a typical 3–8 KB per posting that is 1.5–4 MB travelling DB → Go → JSON → the SSR
   payload → the browser's parser. `sqlc.embed(jobs)` also pulls the `enrichment` JSONB, which
   is then decoded fail-loud for all 500 rows.
2. **Five correlated subqueries per row** in `ListUserJobs` (`user_jobs.sql:255-305`), three of
   them separate passes over `emails` for the same `(user_id, job_id)`.
3. **No index serves those subqueries.** `emails` has `(user_id, received_at DESC) WHERE
   deleted_at IS NULL`; `cvs` has no `(user_id, job_id)` index at all.
4. **`ORDER BY` over a `CASE` expression** cannot use an index.

### Measurement first

Min TTFB over several runs, minus the `/health` network floor — the method recorded in
`hire-jobs-list-slow-query` — plus the response size and the share of it that is description
text. This decides whether the fix is about bytes or about queries.

### Planned remedy

- The tracker listing stops reading `description` and `enrichment`: its own query with an
  explicit column list and a light card projection. `jobview` stays untouched for the three
  surfaces whose contract it is.
- The three `emails` subqueries collapse into one `LATERAL` returning count, newest
  `received_at`, and the pending-suggestion boolean in a single pass — which also removes two
  spellings of one predicate, the cleanup `#1462` was doing elsewhere.
- Two indexes: `emails (user_id, job_id) WHERE deleted_at IS NULL` and `cvs (user_id, job_id)`.

No materialized view or denormalized card table. Both add a second source of truth and a
synchronization path, and the stated priority for this work is that it stays easy to maintain.
If the measurement shows the query — not the payload — dominates even after the LATERAL and the
indexes, materialization gets its own proposal with the number that justifies it.
