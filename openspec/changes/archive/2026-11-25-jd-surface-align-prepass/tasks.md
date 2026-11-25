## 1. Preferred surfaces from JD text

- [x] 1.1 Add a pure helper that, given vacancy plain text, returns canonical → preferred surface (longest alias hit when several; sole hit even if short; JD casing), backed only by skilltag alias tables
- [x] 1.2 Unit tests: both `IaC` and `infrastructure as code` → longest form; JD-only `IaC` → `IaC`; unknown jargon ignored; casing preserved from JD

## 2. Document rewrite (thorough)

- [x] 2.1 Apply preferred surfaces to a `cv.Document` with two tiers: chips/stacks via `Canonicalize` (any alias); summary/bullets via unambiguous phrase/acronym aliases only; word-boundary replace
- [x] 2.2 Collapse skills-group items that become identical after rewrite
- [x] 2.3 Chip/stack tests: `IaC` expands; long form shrinks to JD `IaC`; `Go` chip → `Golang` when JD prefers it; no skill invented when CV lacks the canonical; duplicate `IaC` + long form collapse to one
- [x] 2.4 Prose safety tests (must be thorough): `IaC` / `k8s` / `infrastructure as code` in bullets replace; `go`, `react`, and other `ambiguousWords` in bullets do **not** replace even when JD prefers `Golang`/`React`; 1–2 letter tokens in prose stay; substring embeds (`reaction`, `going`) stay; surrounding words unchanged
- [x] 2.5 Idempotence test: applying twice leaves the document unchanged
- [x] 2.6 Family fixture must **not** rewrite: `pgvector` vs `vector-databases` stay distinct (Phase 1 guard)

## 3. Wire into tailor bootstrap

- [x] 3.1 On create of a new tailored copy, align the document then store it (`CreateTailored`); no model turn has run yet
- [x] 3.2 Repeated bootstrap for an existing copy does not re-align
- [x] 3.3 Add `tailorPrompt` sentence: surfaces already aligned; do not rename for wording
- [x] 3.4 Integration test: fresh mint stores JD form; second bootstrap leaves a user-edited acronym in place

## 4. Reset-from-résumé

- [x] 4.1 After rebuilding from seed, commit surface align through `cvedit` against the bound vacancy
- [x] 4.2 Integration test: reset of a copy whose seed says `IaC` stores the vacancy's preferred form

## 5. Wire into autopilot start

- [x] 5.1 Before the unattended turn's first model call, commit surface align through `cvedit` as its **own** revision (not the run's edit batch); idempotent when already aligned
- [x] 5.2 Autopilot brief states surfaces are already aligned
- [x] 5.3 Integration test: autopilot start rewrites `IaC` before the turn; already-aligned is a no-op
- [x] 5.4 Integration test: undoing the autopilot run reverts the run's edits and **leaves** the align revision (JD wording remains)

## 6. Guardrails

- [x] 6.1 Confirm Phase 1 tests do not encode family behaviour (task 2.6 stays red if someone "helpfully" links those slugs)
- [x] 6.2 `go test` for touched packages; `go vet -tags=integration ./...` before push

## Future phases (separate changes — do not implement here)

- Phase 2: skill families (core ↔ related members) for dedup/match — see proposal/design
- Phase 3: family-aware ensure + skills-chip dedup
- Phase 4: literal coverage diagnostic / `from`→`to` receipt on rendered text
