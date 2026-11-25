## Why

PR #1737 (`feat/profile-contacts-experience-coverage`) fails three independent CI gates after the latest push: web ESLint, Go format check, and design-system token baseline. Unblocking the PR needs a small hygiene pass with no product behaviour change; do not push until the user asks.

## What Changes

- Silence the sole web lint **error**: `svelte/no-at-html-tags` on `{@html renderMarkdown(...)}` in `MatchAnalysisFull.svelte`, matching the established DOMPurify-sanitized pattern used by `AssistantChat.svelte`.
- Run `gofmt` on the three dirty Go files reported by CI (`internal/experience/store_test.go`, `internal/handler/assistant_profile_tool.go`, `internal/handler/me_experience_test.go`).
- Restore design-system token ratchet: update `web-token-baseline.json` (or remove the new arbitrary Tailwind values) for the regressions in `web/src/lib/assistant/presets.ts` and `web/src/routes/my/+layout.svelte`.
- Leave pre-existing oxlint **warnings** alone unless a file we already touch is trivial to clean; they do not fail CI.
- No remote push as part of this change.

## Capabilities

### New Capabilities

<!-- none — CI hygiene only; skip_specs: true -->

### Modified Capabilities

<!-- none — no requirement-level behaviour change -->

## Impact

- `web/src/lib/components/MatchAnalysisFull.svelte` (eslint directive only; rendering already goes through `renderMarkdown` → DOMPurify).
- Go sources listed above (whitespace / formatting only).
- `design-system/scripts/web-token-baseline.json` and/or the two web files that introduced arbitrary Tailwind values.
- Local verification: `pnpm run lint` in `web/`, `gofmt -l` on touched Go paths, `pnpm check:tokens` in `design-system/`.
- Delivery: commit on the feature branch when asked; **do not push** until the user explicitly requests it.
