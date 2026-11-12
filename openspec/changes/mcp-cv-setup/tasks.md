## 1. Shared upload use case
- [x] 1.1 Extract `resumeHandlers.storeCV` (Put + deriveResumeArtifacts); `PutResume` and
      `ExtractResumeProfile` call it, behaviour unchanged (existing tests stay green).

## 2. `add_cv`
- [x] 2.1 Unit tests first: bounds, already-has-CV refusal, storage disabled, stored as
      text/plain with suggestions returned, description names no other tool.
- [x] 2.2 Implement the tool (`assistant_cv_add_tool.go`).

## 3. `get_setup_status`
- [x] 3.1 Unit tests first for the step order and `next_step`.
- [x] 3.2 Implement the tool (`assistant_setup_status_tool.go`).

## 4. Wiring
- [x] 4.1 Register both grant-only in `mcpAccountTools`; classify their effects in
      `TestMCPAccountTools_DeclareTheirEffect`; key tools exclude them.
- [x] 4.2 Instructions line and `set_up_my_profile` prompt; keep the 720-character budget.
- [x] 4.3 Integration test over a real grant: setup status → add_cv → status → refusal.

## 5. Docs + ship
- [x] 5.1 `internal/api/mcpapp/AGENTS.md`.
- [ ] 5.2 gofmt, vet (incl. integration tag), tests, size check, PR.

## 6. Review fixes
- [x] 6.1 `add_cv` takes only the person's own CV, verbatim; no composed history anywhere.
- [x] 6.2 Honest wording on the skills merge the import performs, as on the website.
- [x] 6.3 Atomic first-CV admission: `resume.Store.PutIfAbsent` under a per-user advisory
      lock every upload takes; concurrent integration test.
- [x] 6.4 `minLength` in the input schema.
- [x] 6.5 `next_step` never pushes the optional steps; search once the core is done.
