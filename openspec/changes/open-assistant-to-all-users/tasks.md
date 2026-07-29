## 1. Backend — open the routes

- [x] 1.1 Invert the rollout cases in `internal/handler/assistant_integration_test.go`: the test asserting a signed-in non-member gets `403` now asserts the request is served, and the Bearer case asserting a caller outside the rollout is refused becomes a case asserting they are served over Bearer too. Run them and watch them fail (RED).
- [x] 1.2 Drop `requireRollout` and its `gate` wiring from `register` in `internal/handler/assistant.go`, leaving `mw.key` on every route. Tests from 1.1 go green.
- [x] 1.3 Confirm the unauthenticated-caller test still passes unchanged — authentication must not have loosened with the gate.
- [x] 1.4 Rewrite the route comment in `assistant.go` that explains the restricted rollout, and check no other Go file references `requireRollout`.

## 2. Frontend — remove the dead mirror

- [x] 2.1 Remove the `requireBeta` prop, its type entry, the `allowed` derivation and the `{:else if !allowed}` restricted-rollout branch from `web/src/lib/assistant/AssistantChat.svelte`; drop the now-unused `currentUser` import if nothing else uses it, and unguard the `onMount` boot.
- [x] 2.2 Remove `requireBeta={false}` from `web/src/routes/tailor/[slug]/+page.svelte`.
- [x] 2.3 Replace the stale justification comment above the `/my/assistant` entry in `web/src/lib/accountNav.ts` — the agent runs in our backend, not on the user's machine.
- [x] 2.4 Run the web unit tests and `eslint`; both must be green (the repo gates CI on eslint).

## 3. Documentation

- [x] 3.1 Update the assistant line in the root `AGENTS.md` conventions list — it describes the assistant as gated to the restricted rollout.
- [x] 3.2 Update `internal/handler/AGENTS.md`, which documents `requireRollout` as part of the assistant's middleware chain.

## 4. Verify and finish

- [x] 4.1 `go build ./... && go vet ./... && go test ./...` green; run the assistant integration tests under their build tag.
- [x] 4.2 Invoke the `simplify` skill over the whole diff; tests stay green after it.
- [x] 4.3 Request code review on the full diff, act on Critical and Important findings.
- [ ] 4.4 Open the follow-up for AI-credit metering of assistant turns, so the debt this change takes on is recorded somewhere other than the design doc.
