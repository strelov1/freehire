The approved design lives in
`docs/superpowers/specs/2026-08-01-readme-product-features-design.md`. It is not
duplicated into `design.md` — one document, one source of truth.

## 1. Inventory and verification harness

- [ ] 1.1 Enumerate every production feature from `web/src/routes`, the handler
  route registration, and `internal/` — producing, per feature, its live route,
  its owning packages, and its `AGENTS.md` (if any). Drop anything not reachable
  in production rather than hedging it.
- [ ] 1.2 Write the link check as a runnable command: every `Deep dive:` target
  must exist on disk and every `Live:` route must resolve to a directory under
  `web/src/routes`. It must fail loudly on a bad path — this is the change's
  substitute for a test suite.

## 2. docs/features.md

- [ ] 2.1 Write the Find section — faceted search, collections and role
  landings, saved searches with subscription digests, market analytics, the
  ghost-job signal (as a signal, not a verdict).
- [ ] 2.2 Write the Apply section — CV builder and templates, the deterministic
  CV↔vacancy score and Job Match tab, LLM fit analysis, CV tailoring and
  autopilot, the experience bank and its provenance rule, tracer links, PII
  masking, referrals.
- [ ] 2.3 Write the Track section — the application board and its stages, the
  hosted mail inbox and automatic reply linking, the application event ledger,
  reminders and notification channels.
- [ ] 2.4 Write the Ask section — the in-process assistant, its five presets
  (`chat`, `browse`, `profile`, `tailor`, `interview`), dictation, and the fact
  that tools act as the authenticated caller rather than under a minted
  credential.
- [ ] 2.5 Write the Build on it section — the keyless public API, the CLI, the
  browser extension, ChatGPT Actions, crowdsourced board contributions.
- [ ] 2.6 Mark every LLM-gated feature as requiring `LLM_*`, matching how the
  README already frames optional OAuth credentials.

## 3. README.md

- [ ] 3.1 Add the `## Beyond the catalogue` section after "Why freehire?" — a
  five-row table (Find / Apply / Track / Ask / Build on) linking into
  `docs/features.md`.
- [ ] 3.2 Add `Features` to the nav line.
- [ ] 3.3 Add the load-bearing product packages to the `## Layout` tree.
- [ ] 3.4 Widen the last "Why freehire?" bullet to name the workspace.

## 4. Verify

- [ ] 4.1 Run the link check from 1.2 — zero failures.
- [ ] 4.2 Confirm the size budget: `docs/features.md` under ~160 lines,
  `README.md` under 420.
- [ ] 4.3 Confirm `git diff --stat` touches only `README.md`,
  `docs/features.md`, and this change's own artifacts.
- [ ] 4.4 Request code review on the diff; fix Critical and Important findings.
