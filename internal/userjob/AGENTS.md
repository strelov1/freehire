# Per-user job tracking conventions

## Scope
Per-user job interactions: view/apply/save/track endpoints backed by the `user_jobs` table.

## Always true
- **One row per (user, job).** The composite PK `PRIMARY KEY (user_id, job_id)` is the dedup key — the invariant is "at most one interaction per (user, job)".
- **All writes are idempotent upserts** behind `RequireAuthOrKey` (session cookie or API key) and addressed by the job's public `:slug` (resolved to internal id before the write).
- **`stage` is a controlled vocabulary** (`userjob.Stages`/`ValidStage` in `internal/userjob/stages.go`): applied/screening/responded/interview/offer/accepted/rejected/withdrawn/expired. The SPA mirrors it; an unknown stage is a 400 before any DB touch.
- Handlers return `{"data": interaction}` with `user_id` omitted; public job reads stay unauthenticated.
- `internal/db/user_jobs_stage_integration_test.go` covers the stage vocabulary and interactions.

## How it works

The per-user job-tracking use cases — **`RecordView`** (touches `viewed_at`), **`MarkApplied`** (sets `applied_at`; runs in a transaction that takes `LockJobForApply` first, so concurrent applies serialize and `jobs.applied_count` cannot double-bump — same pattern as `LockJobForVote` on the vote path), **`SaveJob`/`UnsaveJob`** (toggles the saved mark), **`TrackJob`** (sets application `stage` and/or `notes`) — live in **`internal/jobtracking`** (Fiber/pgx-free service; the HTTP handlers in `internal/handler/user_jobs.go` translate the wire format). The SPA records views silently — failures are swallowed and must not break the page.

`internal/userjob` itself keeps the shared tracking vocabulary, as **four tables keyed on one list of stages**, each answering a different question about them, and each bound to that list by a test:

| file | question | guard |
|---|---|---|
| `stages.go` | which stages exist (`Stages`, `ValidStage`) | — |
| `pipeline.go` | how far along is it, and is it settled (`activeRank`, `terminalStages`, `Forward`, `IsTerminal`) | `TestEveryStageIsRankedOrTerminal` |
| `silence.go` | how long may it go quiet (`silenceThresholds`) | `TestSilenceThresholdsCoverExactlyTheActiveStages` |
| `groups.go` | what coarse state does it show as, and what is it called (`Groups`, `GroupOf`, `Label`) | `TestEveryStageBelongsToExactlyOneGroup` |

`counts.go` folds per-stage rows into the pipeline snapshot (`CountByStage`).

**The labels and the group membership live here rather than in the SPA, and that is the point of `groups.go`.** They used to be restated in four frontend places — `board.ts`'s `STAGE_COLUMN`, the seven pipeline buckets, the funnel's own list, and a fourth copy inside `HomeFunnel.svelte` — so one settled application read as `Rejected` in the drawer, `Closed` on the board and `rejected` in a bucket, while two bucket names (`in_progress`, `declined`) appeared nowhere else in the product. All four now derive from `STAGE_GROUPS` / `STAGE_LABELS`, emitted by `cmd/gen-contracts`, with a `satisfies`-style check in the required `pnpm run check` gate. There is a second reader that is not a browser — the in-app assistant calls `internal/jobtracking` directly and never passes through Fiber — which is why the words are Go's and not TypeScript's.

The four groups are `applied` (applied/screening/responded), `interview`, `offer`, and `closed` (accepted/rejected/withdrawn/expired). A card in `Closed` still carries its own stage label, so the coarse column and the precise outcome are legible together; dropping a card there asks which outcome applies, because the group does not determine it.

**There is no `buckets.go` any more.** It held a seven-value vocabulary (`no_answer`, `in_progress`, `interviewing`, `offer`, `accepted`, `rejected`, `declined`) that was a third name for one state. `GET /me/tracking/pipeline` now returns per-stage counts and no `buckets` object — a breaking change made deliberately rather than deprecating the field, since a deprecated field is the third vocabulary surviving in the code and the docs.

**Silence thresholds carry their provenance at the point of definition.** Five specific numbers read as measurement whether or not they are one, and only two of these are: `applied` 21 is measured over 92 observed applications, `interview` 12 over six, `screening` 18 and `responded` 15 are interpolation stepping evenly between those anchors, and `offer` 5 is judgement — no application in the sample has ever reached that stage. Keep the per-value comments when tuning; a bare table invites the next reader to trust all five equally.

Two invariants worth knowing before touching it: a threshold is the **last tolerated day**, not the first offending one, and a settled application reports **no state at all** rather than `active` — the board must be able to tell "nothing owed" from "owed and answered promptly". `TestSilenceThresholdsGrowStricter` guards the ladder's direction, which no individual scenario would catch if it inverted.

**Every movement is also written to `application_events`, and that ledger — not these columns — is what the aggregates read.** The columns answer "where is this application now"; the ledger answers "what happened to it, and when". Three facts were being lost to overwriting before it existed: `stage` has no transition date, `followed_up_at` holds one chase so the second erases the first, and a deleted email removed the only record that a reply arrived. Emission rides inside the statements that already decide the change — `MarkJobApplied` writes its `applied` event under the same predicate that bumps `applied_count`, `TrackJob` writes `stage_set` only when the stage actually moves — so the two records of one transition cannot drift. The vocabulary and the day-math trust rule live in `internal/appevent`; only mail-sourced events carry a date an employer set, so `stage_set` is recorded from day one and read by no timing yet.

**The ledger is append-only with exactly one sanctioned exception: `RedateApplication` updates an `applied` event's `occurred_at`.** Migration 0062 states the append-only rule and it still holds for everything else — a retraction stamps rather than deletes, a second chase is a second row. Re-dating is not a new fact but a repair of the one already recorded: the event says when the person applied, and correcting `applications.applied_at` without it would leave the card reading one month and every aggregate the other, since the aggregates read `occurred_at`. It writes no second `applied` event and never touches `recorded_at`, so "when it happened" and "when we learned of it" stay distinguishable.

**`user_jobs.followed_up_at` records the candidate chasing, and is deliberately outside the clock.** `last_activity_at` is `GREATEST(applied_at, newest linked inbound mail)` — it measures how long the *other side* has been quiet, and a follow-up the candidate sends is not a reply. Adding the column to that `GREATEST(...)` would clear the badge at the moment it matters most, so it is carried *beside* the silence verdict (`jobtracking.TrackedJob`, the listing response, and `GET /me/tracking/:slug`) and never inside its inputs — `SilenceStateFor` is not given it and so cannot read it. `TestAFollowUpDoesNotStopTheSilenceClock` fails if a future reader wires it in; the column comment in migration 0059 says why.

A chased application therefore has **two readings at once**, and the board card shows both: the amber "24d" badge (they still have not answered) plus "chased 2d ago" (we already prodded). The follow-up offer itself stays available — a second chase is the candidate's call. `GET /me/tracking/:slug/followup` assembles the draft via `internal/followup` (deterministic, no LLM, no credits) and refuses anything whose state is not `silent`, reusing `SilenceStateFor` so the offer and the badge can never disagree; `POST` on the same path records the chase. Nothing in this path sends mail — `inboxHandlers` holds no mail client at all.

The `/me/tracking` read joins the caller's interactions with the jobs they touch. View history = all rows; applications = `applied_at IS NOT NULL`.

**The listing serves a CARD, not the posting** (`jobview.Card`). It carries what a row draws — employer, role, the stated facets, skills, collections, the effective posting date, and a `blurb` cut server-side — and the query reads only those columns. Embedding the whole `jobs` row cost 2.37 MB of a 2.83 MB response over 500 applications, for description text no row renders, plus the TOAST fetch per row that reading it implies. The full public job view stays on `GET /me/tracking/:slug`, which the application panel already calls for its linked mail, so the description arrives on a request that was happening anyway. `TestMeasureBoardLoad` holds the line: it fails if a description reappears in the listing or the payload crosses its ceiling.

Two read-time signals are absent from the card by the same reasoning that they were absent before it: `Reality` and `Ghost` are attached by explicit calls the tracking path has never made, so the rows that render cards showed no such badge either way.

## Limitations
- No bulk operations (e.g. "mark all viewed"); each interaction is an individual per-(user, job) upsert.
