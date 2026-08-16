## 0. Review fixes (found by `/code-review` after tasks 1-5 first went green)

- [x] 0.1 **Confirmed bug**: `registry()`'s `viaCookie` gate left an API key able to
      register `read_current_page` for a browse session (`!viaCookie` is also true for
      a Bearer API key, not only the extension's Bearer session JWT) — contradicted
      design.md's own stated rationale. Fixed by combining `!auth.ViaCookie(c) &&
      !auth.ViaAPIKey(c)` into one `asExtension` bool at the single call site.
- [x] 0.2 **Confirmed bug**: the system prompt was still chosen from raw `sess.Preset`
      (always the browse prompt, which opens by insisting the model call
      `read_current_page` first) while only the tool registry was demoted for a
      cookie-authenticated browse turn — the model's first call then failed with
      "unknown tool". This is exactly the split-brain `assistant.NormalizePreset`'s doc
      comment warns against. Fixed by extracting `effectivePreset(preset, asExtension)`
      (`internal/handler/assistant.go`) as the ONE place that resolves a browse turn's
      real preset, called once in `streamSSE` before either the prompt or the registry
      is built; `registry()`'s signature reverted to its original two args and no
      longer knows about the carrier at all. See design.md Decision 3.
- [x] 0.3 Updated the stale invariant in `internal/assistant/AGENTS.md` ("a browse
      session is one held from the browser extension") to state the carrier gate.
- [x] 0.4 Noted, deliberately NOT acted on: `internal/browsertools.Hub` has no shared
      enforcement point of its own, so a future second tool on the same channel would
      need this same carrier check re-applied by hand. Recorded as an accepted risk in
      design.md rather than building an enforcement abstraction for one caller.
- [x] 0.5 Updated the pre-existing (not-new-to-this-change)
      `TestSessionListSpansBrowsingConversations` in
      `internal/handler/assistant_integration_test.go`, which asserted the OLD
      behaviour (browse sessions listed) and only surfaces when the full
      `-tags=integration` suite runs — renamed to
      `TestSessionListSpansRehearsalsButExcludesBrowsing` and inverted its browsing
      assertion.
- [x] 0.6 New regression tests: `TestEffectivePresetDemotesBrowseWithoutTheExtension`,
      `TestEffectivePresetLeavesOtherPresetsAlone`,
      `TestBrowsePresetOverCookieRunsAsAnOrdinaryChat` (asserts prompt AND tools agree)
      in `assistant_preset_test.go`. Full `go test ./...`,
      `go test -tags=integration ./internal/db/` and `./internal/handler/`, and
      `npx vitest run` (web, full suite) all clean beyond the one pre-existing
      unrelated `TestExtractResumeProfile_PDF` failure.
- [x] 0.7 **Confirmed nit, second `/code-review` pass**: `streamSSE` still passed the
      original `sess` (untouched `Preset`) to `boundRunner`, so a demoted browse turn's
      LLM spend was tagged `"preset:browse"` even though it ran the chat prompt with no
      page tool — polluting that cost bucket with turns that never touched the tool the
      tag exists to price. Fixed: `boundRunner` now takes `turnSess`. `UserID` is the
      only other field it reads, unchanged between the two, so no other behavior moves.
      Not independently unit-tested — asserting the exact gateway tag string would need
      mocking `llmkey`/`llm.Client` binding beyond what any existing test does for
      marginal return; covered instead by `go vet`/`go build` plus the same
      `turnSess`-derivation path `TestBrowsePresetOverCookieRunsAsAnOrdinaryChat` already
      pins.
- [x] 0.8 **Confirmed bug, second `/code-review` pass, user directed it into this
      PR**: `POST /me/autofill/run` (`RunAgentAutofill`) attaches to the same
      `internal/browsertools.Hub` channel unconditionally for any `mw.key`-authenticated
      caller (cookie or API key) — the identical leak this whole change closes for
      `read_current_page`, except this one WRITES into the attached browser's form.
      Fixed: refuses with 403 unless `auth.IsExtensionBearer(c)` (see 0.9). Also
      corrected the route's own stale comment
      (`internal/handler/autofill_profile.go`) and the `extension-autofill` spec's
      "accept an API key" line, both of which were already wrong about this endpoint's
      real caller (the extension's Bearer session JWT, per `extension/lib/freehire.ts`
      / `extension/lib/auth.ts` — traced to confirm before writing the fix). New test:
      `TestRunAgentAutofillRefusesNonExtensionAuth`
      (`internal/handler/autofill_agent_test.go`), RED (panic on nil DB fields,
      confirming no guard existed) before the fix, GREEN after. See design.md
      Decision 5 and its accepted-risk note on what this test does NOT cover.
- [x] 0.9 **Third `/code-review` pass, three findings, all fixed**:
  - Public API docs (`web/src/lib/docs/api-spec.ts`) still said `/me/autofill/run` was
    `cookie-or-key` with a `fhk_…` (API-key-shaped) curl example — now a new `'extension'`
    Auth variant, labeled "Browser extension only", with a curl example using a
    placeholder session token instead of the API-key prefix. Regenerated
    `docs/API.md` (`npm run gen:api-docs`); `gen:api-docs:smoke` still passes.
  - `!auth.ViaCookie(c) && !auth.ViaAPIKey(c)` was duplicated (as its De Morgan
    negation) between `streamSSE` and `RunAgentAutofill` — exactly the drift
    design.md's own accepted-risk note anticipated, except it had already happened
    with only two callers, not three. Extracted `auth.IsExtensionBearer(c) bool`
    (`internal/auth/middleware.go`) as the one definition both now call; RED
    (`TestIsExtensionBearer_TrueOnlyForTheSessionJWT`, compile failure) confirmed
    before adding it.
  - `internal/assistant/store.go`'s `PresetBrowse` constant doc comment still stated
    the pre-fix, unconditional guarantee ("the only preset whose agent can see the
    page") — updated to name the carrier gate and point at `effectivePreset`.
  - `gofmt`/`go vet`/`go vet -tags=integration` clean; `go test ./...` clean beyond
    the same pre-existing `TestExtractResumeProfile_PDF`; web `npm run check`
    (svelte-check): 0 errors (pre-existing warnings elsewhere, none in touched
    files); `npx vitest run`: 1006/1006.

## 1. Backend: exclude `browse` from the session rail

- [x] 1.1 `internal/db/queries/assistant.sql`: in `ListAssistantChatSessions`, drop
      `'browse'` from the `preset IN (...)` list and update the comment above the
      query to state the browse exclusion and why (mirroring the existing tailoring
      rationale), per `design.md` Decision 1.
- [x] 1.2 Run `make sqlc` to regenerate `internal/db/*.go`; confirm
      `ListAssistantChatSessionsRow`/the generated method compile unchanged in shape
      (only the WHERE clause changes).
- [x] 1.3 Update or add a test asserting `ListAssistantChatSessions` (or its handler,
      `ListAssistantSessions`) omits a `browse` session for its owner, alongside the
      existing tailoring-exclusion coverage. Covers the `assistant-sessions` spec's
      "A browsing session is absent from the rail" scenario.
      Done: `TestListAssistantChatSessionsExcludesTailorAndBrowse` in
      `internal/db/assistant_preset_integration_test.go` — RED against the old
      query, GREEN after the fix.

## 2. Backend: auth-carrier signal

- [x] 2.1 `internal/auth/middleware.go`: add `localsViaCookie` const and `ViaCookie(c
      *fiber.Ctx) bool`, mirroring `localsViaAPIKey`/`ViaAPIKey`.
- [x] 2.2 Set it `true` in `RequireAuth`'s success path and in
      `RequireAuthOrScopedKey`'s cookie branch; leave it unset on the Bearer branch
      (both JWT and API-key).
- [x] 2.3 Add a test in `internal/auth` covering: cookie auth → `ViaCookie` true;
      Bearer session JWT → `ViaCookie` false; Bearer API key → `ViaCookie` false.
      Done: `TestRequireAuth_FlagsCookieAuth` (middleware_test.go),
      `TestRequireAuthOrKey_FlagsCookieAuth`,
      `TestRequireAuthOrKey_JWTBearerIsNotViaCookie`,
      `TestRequireAuthOrKey_APIKeyIsNotViaCookie` (requireauthorkey_test.go).

## 3. Backend: gate `read_current_page` on the carrier

**Superseded by task group 0** after review found this original approach let the prompt
and the tool set disagree (0.2). Left here for history; the final shape is: `registry()`
keeps its ORIGINAL two-arg signature and stays carrier-blind, `effectivePreset` resolves
the carrier-aware demotion once in `streamSSE`, and both the prompt and `registry()` are
built from that single resolved preset. See design.md Decision 3 and tasks 0.1-0.2, 0.6.

- [x] 3.1 (superseded) ~~add a `viaCookie bool` parameter to `registry()`~~ — reverted;
      `registry()` is unchanged from before this whole change.
- [x] 3.2 (superseded) ~~pass `auth.ViaCookie(c)` into `h.registry(...)`~~ — `streamSSE`
      instead computes `asExtension` and passes a preset-overridden `turnSess` to both
      `registry()` and `assistant.SystemPrompt`.
- [x] 3.3 Update every other `registry(...)` call site — reverted to the original
      two-arg call in `assistant_preset_test.go`, `assistant_inbox_preset_test.go`,
      `assistant_interview_tools_test.go`, since `registry()`'s signature no longer
      changed.
- [x] 3.4 (superseded) `TestBrowsePresetOverCookieHasNoPageTool` replaced by
      `TestBrowsePresetOverCookieRunsAsAnOrdinaryChat` (task 0.6), which additionally
      pins the prompt.
- [x] 3.5 Confirm the existing `TestBrowsePresetOffersThePageTool` is unaffected — it
      still calls `registry()` with two args, unchanged from before this change.
      Group done: full `internal/handler`, `internal/auth`, `internal/assistant`,
      `internal/db` (incl. `-tags=integration`), and web suites pass (one pre-existing
      unrelated failure, `TestExtractResumeProfile_PDF`, confirmed present on
      unmodified origin/main too).

## 4. Frontend: stop treating `browse` as rail-openable

- [x] 4.1 `web/src/lib/assistant/presets.ts`: change `opensInRail` to `preset !==
      'tailor' && preset !== 'browse'`; update its doc comment (the "whitelist goes
      stale" rationale no longer applies to `browse`, which is now excluded on
      purpose, matching `tailor`).
- [x] 4.2 `web/src/lib/assistant/presets.test.ts`: update the `browse` expectation
      from `true` to `false`; add/keep a case pinning `tailor` and `browse` as the
      only two presets `opensInRail` refuses.
- [x] 4.3 Confirm `AssistantChat.svelte`'s existing dead-link handling
      (`showSessionRail && !opensInRail(meta.preset)`) needs no further change — it
      already renders the same "chat unavailable" state `tailor` produces.
      Verified by reading; no edit needed. `npx vitest run presets`: 22/22 pass.

## 5. Verify

- [x] 5.1 `gofmt -w` the touched Go files; `go vet ./...`; `go test ./...`.
      Clean; only pre-existing failure `TestExtractResumeProfile_PDF`
      (confirmed present on unmodified origin/main, unrelated to this change).
- [x] 5.2 `go vet -tags=integration ./...`; if any integration test in
      `internal/handler` touches `ListAssistantSessions` or session creation for
      `browse`, run the tagged suite for that package.
      Full `go test -tags=integration ./internal/db/` (35s) and
      `./internal/handler/` (38s) both clean beyond the same pre-existing PDF failure.
- [x] 5.3 `cd web && npm test -- presets` (or the project's usual vitest invocation)
      for the frontend changes.
      `npx vitest run presets`: 22/22 pass.
- [ ] 5.4 Manually confirm in a dev environment: create a `browse` session from the
      extension, confirm it no longer appears in `/my/assistant`'s rail, and confirm
      a direct link to its id on the website shows the dead-link state rather than a
      working chat.
      NOT DONE — needs `make up` + a built/loaded extension + a signed-in browser
      session; left for the user or a follow-up live-verification pass.
