# Fit analysis: verify after autopilot Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Tailor autopilot run a self-check loop against the free deterministic
`job_match` score, and guarantee the cached LLM fit analysis is refreshed once every
autopilot run ends — repealing the "fit analysis is a frozen snapshot of the base
profile" rule.

**Architecture:** Two independent, additive changes around the existing
`PostAssistantAutopilot` handler and the Tailor preset's system prompt. No new tools, no
DB schema change. `internal/matchanalysis`'s three-stage chain itself is untouched.

**Tech Stack:** Go (Fiber v2, sqlc-generated `db.Queries`), the existing `internal/assistant`
tool-calling runner, Svelte 5 for the one copy change.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md`.
- The post-run refresh is **unconditional** — runs every autopilot turn, even with zero
  edits — and **unmetered**: it must never call `credits.Debit`/`debitMatch`.
- No new DB migration. Still one `user_job_analysis` row per `(user, job)`.
- Anything that runs after `PostAssistantAutopilot` opens its SSE stream must use a plain
  `context.Context`, never the request's `*fiber.Ctx` — the fiber ctx is released the
  moment the handler returns, before the stream's writer goroutine runs (see the existing
  comments on `cacheAnalysis` and `StreamMatchAnalysis` in
  `internal/handler/match_analysis*.go`).
- English only in code/comments/commits (root `AGENTS.md`).
- Run `go vet -tags=integration ./...` before considering this plan done — a changed
  handler signature only shows up there, not in plain `go build`/`go test ./...`.
- The design doc's open question about "coverage by the background-entrypoint llmkey
  test" does not apply here and needs no action: `internal/llmkey/scope_test.go` only
  fails a build whose `cmd/*` binary imports `internal/llmkey` directly (background
  workers must use the service credential, never a user's). `prepareAutopilotRun` lives in
  `internal/handler` (the `cmd/server` binary) and already resolves the user's own
  credential the same way `ensureCachedAnalysis`/`runAnalysis` do today — "unmetered"
  here means it must never call `credits.Debit`, which Task 2's implementation simply
  never does.

---

### Task 1: Extract `buildAnalysisInput` from `runAnalysis`

Pure refactor, no behavior change — a prerequisite for Task 2, which needs to assemble a
fit-chain `Input` once and reuse it after the fiber ctx is gone.

**Files:**
- Modify: `internal/handler/match_analysis.go:229-245` (`runAnalysis`)

**Interfaces:**
- Produces: `func (h *matchHandlers) buildAnalysisInput(c *fiber.Ctx, job db.Job, userID int64, profile userprofile.Profile, blockers []hardconstraint.Blocker) matchanalysis.Input` — Task 2 calls this directly.
- `runAnalysis`'s own signature and behavior are unchanged; it now calls `buildAnalysisInput` internally.

- [ ] **Step 1: Extract the input literal into its own method**

Replace:

```go
func (h *matchHandlers) runAnalysis(c *fiber.Ctx, userID int64, job db.Job, profile userprofile.Profile, blockers []hardconstraint.Blocker) (*matchanalysis.Analysis, error) {
	analyzer := h.matchAnalysis.As(h.llm.bind(c.Context(), userID, tagMatchAnalysis))
	return analyzer.Analyze(c.Context(), matchanalysis.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         h.companyInfo(c, job.CompanySlug),
		StructuredResume:    h.candidateProfile(c, userID),
		Match:               jobmatch.Compute(job.Skills, profile.Skills),
		JobWorkMode:         job.WorkMode,
		JobRemote:           job.Remote,
		JobLocation:         job.Location,
		JobRegions:          job.Regions,
		JobCountries:        job.Countries,
		LocationPreferences: string(profile.LocationPreferences),
		Blockers:            blockers,
	})
}
```

with:

```go
// buildAnalysisInput assembles the fit chain's input from the candidate's profile and the
// vacancy. Split out of runAnalysis so a caller that must run the chain from a plain
// context — the autopilot's post-run refresh (Task 2), which runs from the SSE writer's
// detached goroutine after the fiber ctx is gone — can build the Input once, while c is
// still valid, and carry only the plain value into that goroutine.
func (h *matchHandlers) buildAnalysisInput(c *fiber.Ctx, job db.Job, userID int64, profile userprofile.Profile, blockers []hardconstraint.Blocker) matchanalysis.Input {
	return matchanalysis.Input{
		JobTitle:            job.Title,
		JobDescription:      job.Description,
		CompanyInfo:         h.companyInfo(c, job.CompanySlug),
		StructuredResume:    h.candidateProfile(c, userID),
		Match:               jobmatch.Compute(job.Skills, profile.Skills),
		JobWorkMode:         job.WorkMode,
		JobRemote:           job.Remote,
		JobLocation:         job.Location,
		JobRegions:          job.Regions,
		JobCountries:        job.Countries,
		LocationPreferences: string(profile.LocationPreferences),
		Blockers:            blockers,
	}
}

func (h *matchHandlers) runAnalysis(c *fiber.Ctx, userID int64, job db.Job, profile userprofile.Profile, blockers []hardconstraint.Blocker) (*matchanalysis.Analysis, error) {
	analyzer := h.matchAnalysis.As(h.llm.bind(c.Context(), userID, tagMatchAnalysis))
	return analyzer.Analyze(c.Context(), h.buildAnalysisInput(c, job, userID, profile, blockers))
}
```

- [ ] **Step 2: Confirm no regression**

Run: `go test -tags=integration ./internal/handler/ -run TestMatchAnalysisEndpoints -v`
Expected: PASS (behavior-preserving extraction; this test already exercises `runAnalysis`
through `PostMatchAnalysis`).

- [ ] **Step 3: Commit**

```bash
git add internal/handler/match_analysis.go
git commit -m "refactor(matchanalysis): extract buildAnalysisInput from runAnalysis"
```

---

### Task 2: Guaranteed post-run refresh — `prepareAutopilotRun` and the `PostAssistantAutopilot` rewire

The core backend change. Adds a closure-returning preparation step that (a) keeps today's
fill-if-empty pre-run behavior and (b) hands back a function that unconditionally
recomputes and overwrites the cached analysis once the autopilot turn ends.

**Files:**
- Modify: `internal/handler/match_analysis.go` (add `prepareAutopilotRun`, near `ensureCachedAnalysis`)
- Modify: `internal/handler/assistant.go:744-781` (`PostAssistantAutopilot`, replace `ensureAnalysisForRun`)
- Test: `internal/handler/assistant_autopilot_integration_test.go`

**Interfaces:**
- Consumes: `buildAnalysisInput` (Task 1), `runAnalysis`'s existing helpers (`h.userProfile.Get`, `h.jobBlockers`, `h.cvUploadedAt`, `h.matchAnalysis.As`, `h.llm.bind`, `h.cacheAnalysis`), `ensureCachedAnalysis` (unchanged).
- Produces: `func (h *matchHandlers) prepareAutopilotRun(c *fiber.Ctx, userID int64, job db.Job) func(context.Context)` — called once by `assistantHandlers.prepareAutopilotAnalysis`, which Task 3+ do not touch.

- [ ] **Step 1: Rewrite the existing "must not recompute" test to assert the new "always refreshes" behavior**

This test's current premise (`fitM.n != 0` after a run means failure) is exactly what
this task repeals — a cached analysis must now ALWAYS be recomputed at the end of a run.
Replace the whole `TestAutopilotReusesAnExistingCachedAnalysis` function in
`internal/handler/assistant_autopilot_integration_test.go` with:

```go
// TestAutopilotRefreshesAnalysisAfterEveryRun: the fit analysis is no longer a frozen
// snapshot of the base profile (see docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md).
// An autopilot run must recompute and overwrite the cached (user, job) row once it ends,
// even when a (now-stale) analysis was already cached and the run itself made zero edits.
func TestAutopilotRefreshesAnalysisAfterEveryRun(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	turnM := &turnModel{replies: []*llms.ContentChoice{{Content: "Walked the requirements."}}}
	fitM := &fitModel{resp: []string{fitStage1, fitStage2, fitStage3}}
	an := matchanalysis.NewAnalyzer(llm.NewWithModel(fitM))

	bank := experience.NewStore(experience.NewQueriesRepository(queries))
	h := &assistantHandlers{
		store: assistant.NewStore(queries), queries: queries,
		maxPrompt:  defaultAssistantMaxPrompt,
		stages:     queries,
		experience: bank,
		cv: &cvHandlers{
			cvStore:            cv.NewStore(cv.NewQueriesRepository(queries)),
			editor:             cvedit.NewEditor(cvedit.NewRepository(pool, queries), bankGate{bank: bank}),
			queries:            queries,
			jobReader:          queries,
			matchAnalysisCache: queries,
			match:              fitAPI(pool, queries, iss, resume.New(nil, resume.NewQueriesRepository(queries)), an),
		},
	}
	h.runner = assistant.NewRunner(turnM, h.store, assistant.RunnerConfig{MaxSteps: 3})
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	mw := middleware{
		cookie: auth.RequireAuth(iss, testVersions),
		key:    auth.RequireAuthOrKey(iss, testVersions, apiKeys{queries}),
	}
	h.register(api, mw)
	h.cv.register(api, mw)

	userID, cookie := assistantUser(t, pool, iss, "autopilot-refresh@example.test", true)
	seedBankedCareer(t, queries, userID)
	sessionID, cvID := seedTailoringSession(t, pool, h, userID)

	var jobID int64
	if err := pool.QueryRow(context.Background(), `SELECT job_id FROM cvs WHERE id = $1`, cvID).Scan(&jobID); err != nil {
		t.Fatalf("read job id: %v", err)
	}
	// A stale analysis under a model id the fake never produces, so a later match on the
	// LIVE model id (empty string — llm.NewWithModel sets none) can only mean the row was
	// actually overwritten, not left alone.
	if err := queries.UpsertUserJobAnalysis(context.Background(), db.UpsertUserJobAnalysisParams{
		UserID: userID, JobID: jobID, Analysis: []byte(`{"verdict":"Good Fit","overall_score":70}`), Model: "stale-model",
	}); err != nil {
		t.Fatalf("seed cached analysis: %v", err)
	}

	resp := assistantRequest(t, app, fiber.MethodPost,
		"/api/v1/assistant/sessions/"+sessionID+"/autopilot", cookie, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("autopilot: status %d", resp.StatusCode)
	}
	io.ReadAll(resp.Body)
	resp.Body.Close()

	if fitM.n != 3 {
		t.Errorf("fit model called %d times, want exactly 3 (one full chain run) — a cached analysis must still be refreshed once, not skipped and not recomputed twice", fitM.n)
	}
	row, err := queries.GetUserJobAnalysis(context.Background(), db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatalf("read cached analysis: %v", err)
	}
	if row.Model == "stale-model" {
		t.Error("cached analysis still carries the pre-run model stamp — the row was not overwritten")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test -tags=integration ./internal/handler/ -run TestAutopilotRefreshesAnalysisAfterEveryRun -v`
Expected: FAIL — `fitM.n` is `0` (today's `ensureAnalysisForRun` skips the LLM call
entirely because a row is already cached; there is no post-run step at all).

- [ ] **Step 3: Also lock the cold cache case (`fitM.n` doubles: before AND after)**

In the same file, `TestAutopilotComputesAnalysisWhenMissing` currently only checks that a
row ends up cached. Add, right after its existing `queries.GetUserJobAnalysis` assertion
at the end of the function:

```go
	if fitM.n != 6 {
		t.Errorf("fit model called %d times, want 6 (one full chain run before the run, one after) — the post-run refresh must fire even when the pre-run step just computed one", fitM.n)
	}
```

Run: `go test -tags=integration ./internal/handler/ -run TestAutopilotComputesAnalysisWhenMissing -v`
Expected: FAIL — currently `fitM.n` is `3` (only the pre-run compute happens).

- [ ] **Step 4: Add `prepareAutopilotRun` to `internal/handler/match_analysis.go`**

Add right after `ensureCachedAnalysis` (after line 276):

```go
// prepareAutopilotRun ensures the fit analysis is cached before an autopilot run starts —
// exactly ensureCachedAnalysis's fill-if-empty, so cv_context has something to read — and
// returns the closure PostAssistantAutopilot calls once the run ends, which UNCONDITIONALLY
// recomputes the chain and overwrites the (user, job) cache, even when nothing was cached
// or an analysis was already there. See
// docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md — this is
// what repeals the "fit analysis is a frozen base-profile snapshot" rule.
//
// Both halves share one assembled Input/Analyzer pair so the profile/blockers/bank reads
// happen once per autopilot invocation rather than twice. Built here, while c is valid,
// because the returned closure runs later from the SSE writer's detached goroutine, which
// only has a plain context.Context (see cacheAnalysis's own comment on the same
// constraint). Never debits credits — this path, like ensureCachedAnalysis, is unmetered.
func (h *matchHandlers) prepareAutopilotRun(c *fiber.Ctx, userID int64, job db.Job) func(context.Context) {
	h.ensureCachedAnalysis(c, userID, job)

	profile, _ := h.userProfile.Get(c.Context(), userID)
	blockers := h.jobBlockers(c.Context(), userID, job, profile)
	analyzer := h.matchAnalysis.As(h.llm.bind(c.Context(), userID, tagMatchAnalysis))
	input := h.buildAnalysisInput(c, job, userID, profile, blockers)
	cvUploadedAt, _ := h.cvUploadedAt(c, userID)

	return func(ctx context.Context) {
		analysis, err := analyzer.Analyze(ctx, input)
		if err != nil {
			log.Printf("matchanalysis: post-autopilot refresh, user %d job %d: %v", userID, job.ID, err)
			return
		}
		if analysis == nil {
			return // LLM unconfigured — nothing to cache
		}
		h.cacheAnalysis(ctx, userID, job, cvUploadedAt, analysis)
	}
}
```

- [ ] **Step 5: Replace `ensureAnalysisForRun` and rewire `PostAssistantAutopilot` in `internal/handler/assistant.go`**

Replace the `ensureAnalysisForRun` method (lines 763-781) with:

```go
// prepareAutopilotAnalysis resolves the run's vacancy and delegates to
// matchHandlers.prepareAutopilotRun. A missing match surface or job lookup failure
// degrades to a no-op refresh — the same best-effort posture the pre-run ensure step
// already had before this change.
func (h *assistantHandlers) prepareAutopilotAnalysis(c *fiber.Ctx, sess assistant.Session) func(context.Context) {
	noop := func(context.Context) {}
	if h.cv == nil || h.cv.match == nil {
		return noop
	}
	job, err := h.queries.GetJob(c.Context(), *sess.JobID)
	if err != nil {
		log.Printf("assistant: loading job %d for autopilot's analysis: %v", *sess.JobID, err)
		return noop
	}
	return h.cv.match.prepareAutopilotRun(c, sess.UserID, job)
}
```

Then replace the body of `PostAssistantAutopilot` (lines 758-760):

```go
	h.ensureAnalysisForRun(c, sess)
	h.layDownRunPlan(c.Context(), sess)
	return h.streamTurn(c, sess, autopilotBrief, assistant.TurnConfig{MaxSteps: autopilotMaxSteps})
```

with:

```go
	refreshAnalysis := h.prepareAutopilotAnalysis(c, sess)
	h.layDownRunPlan(c.Context(), sess)
	return h.streamSSE(c, sess, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		err := runner.Run(ctx, sess, reg, system, autopilotBrief, assistant.TurnConfig{MaxSteps: autopilotMaxSteps}, emit)
		refreshAnalysis(ctx)
		return err
	})
```

(`streamTurn` stays as-is and stays in use by `PostAssistantOpening` — only the autopilot
path now supplies its own `start` closure to `streamSSE` so it can run code after
`runner.Run` returns, inside the same detached goroutine.)

- [ ] **Step 6: Run both tests to verify they pass**

Run: `go test -tags=integration ./internal/handler/ -run 'TestAutopilotRefreshesAnalysisAfterEveryRun|TestAutopilotComputesAnalysisWhenMissing' -v`
Expected: PASS on both.

- [ ] **Step 7: Run the full autopilot integration suite for regressions**

Run: `go test -tags=integration ./internal/handler/ -run TestAutopilot -v`
Expected: PASS on all (`TestAutopilotRunsOnATailoringSessionAndSnapshotsFirst`,
`TestAutopilotIsRefusedOnANonTailoringSession`, `TestAutopilotOnAForeignSessionIsNotFound`,
`TestAnAutopilotRunSearchesEditsAndReports`, `TestARunThatNeverReportsStillLeavesOne`).

- [ ] **Step 8: `go vet` the whole module (catches any other caller of the old method name)**

Run: `go vet -tags=integration ./...`
Expected: clean.

- [ ] **Step 9: Commit**

```bash
git add internal/handler/match_analysis.go internal/handler/assistant.go internal/handler/assistant_autopilot_integration_test.go
git commit -m "feat(matchanalysis): refresh the cached fit analysis after every autopilot run"
```

---

### Task 3: Self-check loop — prompt change

Instructs the tailoring agent to call the existing, already-wired `job_match` tool before
finishing an unattended run, and to keep editing while something closeable remains.

**Files:**
- Modify: `internal/assistant/prompt.go:152-162` (`tailorPrompt`'s `UNATTENDED RUNS` section)
- Test: `internal/assistant/prompt_test.go`

**Interfaces:**
- Consumes: nothing new — `job_match` (`internal/handler/assistant_cv_tools.go:210`) and
  `cv_context`'s `missing_have`/`missing_gap` vocabulary already exist and are already
  registered for the Tailor preset.
- Produces: nothing new — this is a prompt-content-only change.

- [ ] **Step 1: Write the failing test**

Add to `internal/assistant/prompt_test.go`:

```go
// TestTailorPromptSelfChecksWithJobMatchBeforeReporting: an unattended run must verify its
// own edits against the deterministic job_match score before it reports — the agent
// cannot be trusted to know whether an edit actually reads as closing a requirement (see
// docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md).
func TestTailorPromptSelfChecksWithJobMatchBeforeReporting(t *testing.T) {
	p := SystemPrompt(PresetTailor)

	unattendedIdx := strings.Index(p, "UNATTENDED RUNS")
	if unattendedIdx == -1 {
		t.Fatal("the tailoring prompt lost its UNATTENDED RUNS section")
	}
	unattended := p[unattendedIdx:]

	for _, want := range []string{"job_match", "missing_have", "missing_gap", "tailor_report"} {
		if !strings.Contains(unattended, want) {
			t.Errorf("the unattended-run section never mentions %q; the agent has no instruction to verify its own edits before reporting", want)
		}
	}
	if strings.Index(unattended, "job_match") > strings.Index(unattended, "tailor_report") {
		t.Error("job_match must be checked BEFORE tailor_report, not after — the report should reflect what the check found")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/assistant/ -run TestTailorPromptSelfChecksWithJobMatchBeforeReporting -v`
Expected: FAIL — `missing_have`/`missing_gap` do not appear inside the `UNATTENDED RUNS`
section today (only in the earlier, attended-mode part of the prompt).

- [ ] **Step 3: Add the self-check bullet**

In `internal/assistant/prompt.go`, inside the `UNATTENDED RUNS` block, insert a new bullet
between the existing "Ask NOTHING while you are running." bullet and the "Finish by
calling `tailor_report` ONCE..." bullet (i.e., between what are today lines 157 and 158):

```go
- Ask NOTHING while you are running. A requirement the bank has nothing for is carried to the report, not turned into a question mid-pass.
- Before you call ` + "`tailor_report`" + `, call ` + "`job_match`" + ` to see what your edits actually produced — not what you believe they produced. If it still shows a closeable ` + "`missing_have`" + ` or ` + "`missing_gap`" + `, edit again and check once more; two or three rounds of this is normal. Stop once nothing closeable is left, or once another round would not change the outcome — do not loop for its own sake.
- Finish by calling ` + "`tailor_report`" + ` ONCE with every requirement you considered: ` + "`closed_bank`" + ` where the bank had evidence and you wrote it in, ` + "`open`" + ` where it had none, ` + "`not_reached`" + ` for anything you did not get to. Copy each requirement's text verbatim from ` + "`cv_context`" + `.
```

(Only the middle line is new; the two around it are unchanged and shown for placement.)

- [ ] **Step 4: Run it to verify it passes**

Run: `go test ./internal/assistant/ -run TestTailorPromptSelfChecksWithJobMatchBeforeReporting -v`
Expected: PASS

- [ ] **Step 5: Run the full assistant package test suite**

Run: `go test ./internal/assistant/...`
Expected: PASS (confirms `TestTailorPromptDescribesTheUnattendedRun` and every other
prompt test still holds with the new bullet present).

- [ ] **Step 6: Commit**

```bash
git add internal/assistant/prompt.go internal/assistant/prompt_test.go
git commit -m "feat(assistant): have the tailoring autopilot verify edits with job_match before reporting"
```

---

### Task 4: OpenSpec delta — Job Match tab is no longer a "snapshot"

Updates the live requirement that currently states the cached fit analysis is a frozen
base-profile snapshot — false the moment Task 2 ships.

**Files:**
- Create: `openspec/changes/2026-08-09-fit-analysis-post-autopilot-verify/proposal.md`
- Create: `openspec/changes/2026-08-09-fit-analysis-post-autopilot-verify/specs/tailor-workspace/spec.md`

**Interfaces:** none (documentation only).

- [ ] **Step 1: Write the proposal**

Create `openspec/changes/2026-08-09-fit-analysis-post-autopilot-verify/proposal.md`:

```markdown
## Why

The Job Match tab requirement (`tailor-workspace` spec) states the cached fit analysis is
"a snapshot of the base profile" — frozen on purpose, so it is never mixed with the live
`job_match` score. That was true when the only way to get a fresh fit analysis was the
candidate clicking Recompute. It stops being true once the autopilot run refreshes the
cached analysis itself, every run (see
`docs/superpowers/specs/2026-08-09-fit-analysis-post-autopilot-verify-design.md`).

## What Changes

- The Job Match tab's fit-analysis panel is no longer described as a base-profile
  snapshot — it is kept current by the latest autopilot run.
- No scenario changes: "each tab holds one baseline" still holds (Job Match vs Score
  measure different things); only the wording of what the Job Match tab's second number
  IS changes.

## Impact

- Affected spec: `tailor-workspace`
- Affected code: `internal/handler/match_analysis.go`, `internal/handler/assistant.go`,
  `web/src/lib/tailor/ArtifactPanel.svelte` (implemented in this plan's Tasks 1, 2, 5)
```

- [ ] **Step 2: Write the MODIFIED spec delta**

Create `openspec/changes/2026-08-09-fit-analysis-post-autopilot-verify/specs/tailor-workspace/spec.md`
with the full requirement restated (per this repo's OpenSpec convention — MODIFIED
requirements are given in full, not as a diff — see the archived
`2026-07-31-tailor-job-match-tab/specs/tailor-workspace/spec.md` for precedent), changing
only the one bullet:

```markdown
## MODIFIED Requirements

### Requirement: The workspace is a three-column surface

The workspace SHALL lay out its ready state in three columns: a left panel tabbed between the CV
editor and the chat, a centre column showing the live CV preview, and a right panel tabbed
between templates, the job description, the job match, and the score. The left and right panels
SHALL be width-adjustable via draggable splitters clamped to a sensible range, with the centre
column taking the remaining width.

The right panel's tabs SHALL be divided by what each one measures, and no tab SHALL mix
measurements taken against different baselines:

- **Job Match** — the live job-anchored score of the tailored document against this vacancy, with
  the cached fit analysis beneath it kept current by the latest autopilot run, not a frozen
  snapshot;
- **Score** — the ATS-readability delta of the tailored document against the base CV, and the log
  of the last autopilot run;
- **Job description** and **Templates** — unchanged.

#### Scenario: The three columns render

- **WHEN** the workspace ready state renders on a wide viewport
- **THEN** the left tabbed panel (Editor/Chat), the centre CV preview, and the right tabbed panel (Templates/Job description/Job Match/Score) are all visible side by side

#### Scenario: A side panel resizes and clamps

- **WHEN** the user drags a side-panel splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing, and the centre column absorbs the change

#### Scenario: Each tab holds one baseline

- **WHEN** the user opens the Job Match tab
- **THEN** it shows the score measured against the vacancy, and the ATS-readability delta measured against the base CV is not shown there
```

(`tasks.md`/`design.md` are deliberately omitted — this delta is wording-only and already
fully covered by this plan's Task 2 and the linked superpowers design doc.)

- [ ] **Step 3: Validate with the project's own tooling, if available**

Run whatever this repo's OpenSpec validation command is (check `openspec/AGENTS.md` or
`package.json`/`Makefile` for an `openspec validate`-style script) against the new change
folder. If none exists, skip — this is a documentation-only step with no build to break.

- [ ] **Step 4: Commit**

```bash
git add openspec/changes/2026-08-09-fit-analysis-post-autopilot-verify/
git commit -m "docs(openspec): fit analysis is refreshed by autopilot, not a base-profile snapshot"
```

Merging this delta into the live `openspec/specs/tailor-workspace/spec.md` happens through
this repo's normal archive step once Tasks 1-3/5 have shipped — do not hand-edit the live
spec directly (see the `opsx:archive`/`opsx:sync` skills and this project's OpenSpec
memory notes on archive ordering).

---

### Task 5: Frontend copy — the Job Match tab no longer calls it a snapshot

**Files:**
- Modify: `web/src/lib/tailor/ArtifactPanel.svelte:5-9,58-59,247-256`

**Interfaces:** none — copy and comments only, `MatchAnalysisFull`'s props are unchanged.

- [ ] **Step 1: Update the header comment (lines 1-16)**

Replace:

```
  //  - Job Match — the live, deterministic score of the document being edited against this
  //    vacancy, with the cached LLM fit analysis beneath it, labelled as the snapshot of the
  //    base profile that it is.
```

with:

```
  //  - Job Match — the live, deterministic score of the document being edited against this
  //    vacancy, with the cached LLM fit analysis beneath it — refreshed by every autopilot
  //    run, not a frozen snapshot.
```

- [ ] **Step 2: Update the `autopilotReport` prop doc comment (around line 58-59)**

Replace:

```
    /** The last unattended run's log, shown in the Score tab. The fit analysis is untouched
     *  by a run — it measures the base profile, not this tailored copy. */
```

with:

```
    /** The last unattended run's log, shown in the Score tab. The fit analysis (Job Match
     *  tab) is refreshed by every run — see prepareAutopilotRun on the server. */
```

- [ ] **Step 3: Update the on-screen copy and its explanatory comment (around lines 247-256)**

Replace:

```svelte
        <!-- The fit analysis measures the candidate's BASE profile, not the document being
             edited, and says so. An unlabelled fit score beside a live one teaches the
             candidate that tailoring does not move the number — true of that score, false of
             this surface as a whole. -->
        <div class="mb-3 rounded-lg border border-border bg-muted/30 px-2.5 py-2">
          <h3 class="text-sm font-semibold text-foreground">Fit analysis</h3>
          <p class="mt-0.5 text-xs leading-snug text-muted-foreground">
            A snapshot of your base profile against this vacancy. It does not move as you edit
            this CV — recompute it below to take it again.
          </p>
        </div>
```

with:

```svelte
        <!-- The fit analysis used to be a frozen base-profile snapshot; it is now refreshed
             by every autopilot run, so it says that instead of implying it never moves. -->
        <div class="mb-3 rounded-lg border border-border bg-muted/30 px-2.5 py-2">
          <h3 class="text-sm font-semibold text-foreground">Fit analysis</h3>
          <p class="mt-0.5 text-xs leading-snug text-muted-foreground">
            Refreshed automatically after every autopilot run. Recompute it below any time.
          </p>
        </div>
```

- [ ] **Step 4: Grep for any other leftover "snapshot" copy**

Run: `grep -rn "snapshot" web/src/lib/tailor/ web/src/lib/components/MatchAnalysisFull.svelte`
Expected: no remaining references to the fit analysis being a base-profile snapshot.

- [ ] **Step 5: Visual check**

Run: `cd web && pnpm dev`, open a tailored CV's workspace, select the Job Match tab, confirm
the new copy renders (per this repo's convention of testing UI changes in a real browser
before calling them done — see root `AGENTS.md`).

- [ ] **Step 6: Commit**

```bash
git add web/src/lib/tailor/ArtifactPanel.svelte
git commit -m "fix(tailor): Job Match tab no longer calls the fit analysis a frozen snapshot"
```
