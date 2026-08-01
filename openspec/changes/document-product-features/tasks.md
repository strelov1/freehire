The approved design lives in
`docs/superpowers/specs/2026-08-01-readme-product-features-design.md`. It is not
duplicated into `design.md` — one document, one source of truth.

## 1. Inventory and verification harness

- [x] 1.1 Enumerate every production feature from `web/src/routes`, the handler
  route registration, and `internal/` — producing, per feature, its live route,
  its owning packages, and its `AGENTS.md` (if any). Drop anything not reachable
  in production rather than hedging it.
- [x] 1.2 Write the link check as a runnable command: every `Deep dive:` target
  must exist on disk and every `Live:` route must resolve to a directory under
  `web/src/routes`. It must fail loudly on a bad path — this is the change's
  substitute for a test suite.

  Written as `docs/check-feature-links.sh`, proven against injected faults (a
  dangling `AGENTS.md` target, a non-existent web route, a non-existent server
  route — all three caught), then **deleted rather than committed**. No CI
  workflow runs markdown link checks, so a committed-but-unwired script would
  rot into false assurance. The seam: if `docs/features.md` starts drifting,
  wire a link check into `.github/workflows/ci.yml` as its own change.

## 2. docs/features.md

- [x] 2.1 Write the Find section — faceted search, collections and role
  landings, saved searches with subscription digests, market analytics, the
  ghost-job signal (as a signal, not a verdict).
- [x] 2.2 Write the Apply section — CV builder and templates, the deterministic
  CV↔vacancy score and Job Match tab, LLM fit analysis, CV tailoring and
  autopilot, the experience bank and its provenance rule, tracer links, PII
  masking, referrals.
- [x] 2.3 Write the Track section — the application board and its stages, the
  hosted mail inbox and automatic reply linking, the application event ledger,
  reminders and notification channels.
- [x] 2.4 Write the Ask section — the in-process assistant, its five presets
  (`chat`, `browse`, `profile`, `tailor`, `interview`), dictation, and the fact
  that tools act as the authenticated caller rather than under a minted
  credential.
- [x] 2.5 Write the Build on it section — the keyless public API, the CLI, the
  browser extension, ChatGPT Actions, crowdsourced board contributions.
- [x] 2.6 Mark every LLM-gated feature as requiring `LLM_*`, matching how the
  README already frames optional OAuth credentials.

## 3. README.md

- [x] 3.1 Add the `## Beyond the catalogue` section after "Why freehire?" — a
  five-row table (Find / Apply / Track / Ask / Build on) linking into
  `docs/features.md`.
- [x] 3.2 Add `Features` to the nav line.
- [x] 3.3 Add the load-bearing product packages to the `## Layout` tree.
- [x] 3.4 Widen the last "Why freehire?" bullet to name the workspace.

## 4. Verify

- [x] 4.1 Run the link check from 1.2 — zero failures.
- [x] 4.2 Confirm the size budget: `README.md` is 413 lines (under 420, as
  planned). `docs/features.md` came in at **198 lines against an estimated
  ~150–160** — 19 features at roughly 10 lines each; 181 before the review, plus
  17 lines of corrections the review forced (see 4.4). The estimate was a guess
  made before the inventory ran; the overrun is feature entries and accuracy, not
  padding, and cutting to hit a number chosen before the content was known would
  mean deleting true statements. Recorded rather than met.
- [x] 4.3 Confirm `git diff --stat` touches only `README.md`,
  `docs/features.md`, and this change's own artifacts.
- [x] 4.4 Request code review on the diff; fix Critical and Important findings.

  Reviewed inline rather than by dispatching a reviewer subagent: this session
  carries a standing instruction not to spawn agents unless asked. The review
  was a claim-by-claim check of the prose against the code, which is where the
  risk of a docs change lives. Six findings, all fixed:

  1. **Critical — false claim.** The intro asserted "nothing here is behind a
     paywall". `credits.FeatureMatch` and `credits.FeatureTailor` are metered,
     and `cv_tailor.go:71` blocks tailoring on an insufficient balance. Rewritten
     to describe the monthly points grant and the 402, and both features marked.
  2. **Important — inverted guarantee.** Faceted search was said to make a
     filtered list "complete". Dict-only facets buy precision, not recall: an
     unrecognised term yields no tag, so the posting drops out of the filter. The
     trade is now stated in the direction it actually runs.
  3. **Important — overclaim.** "A job being pruned does not erase your
     application" was true of `applications` (whose `job_id` `cmd/prune` clears)
     but false of `user_jobs` (`ON DELETE CASCADE`, migrations/0001_init.sql:992).
     Scoped to applications, with views and saves called out as not surviving.
  4. **Important — unverified behaviour.** Tracer links were said to invalidate
     tokens when switched off. The handler shows something different and better:
     enabling is refused without a visitor salt, disabling never is.
  5. **Important — unverified behaviour.** Interview rehearsal was said to bank
     what you say; the preset's only tool reads. Replaced with the verified fact
     that a rehearsal binds to an application the server checks for itself.
  6. **Minor — misleading pointers.** Market analytics pointed at
     `internal/viewlog` (nginx logs → per-job views, not insights) and collections
     at `internal/collections` (visa-sponsor registry parsing, not the landings).
     Both repointed at what actually serves the page.

  Re-verified after the fixes: every pointer resolves, and every `internal/`,
  `cmd/` and `web/` path named in the document exists on disk.
