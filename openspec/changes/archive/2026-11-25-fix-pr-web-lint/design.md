## Context

See proposal.md — Why. PR #1737 is red on three independent jobs from the same tip (`5965cf4a`). Product behaviour is already correct; the gates are format/lint/ratchet only. Specs are skipped (`skip_specs: true`).

## Goals / Non-Goals

**Goals:**
- Make `pnpm run lint` (web), `gofmt -l` on changed Go files, and `pnpm check:tokens` (design-system) exit 0 on this branch tip.
- Keep the fit-verdict markdown path DOMPurify-sanitized; only align ESLint with peers.
- Leave changes as local commits until the user asks to push.

**Non-Goals:**
- Broader oxlint warning cleanup across `web/`.
- Changing `renderMarkdown` / DOMPurify policy.
- Teaching the token detector about regex character classes in general (optional follow-up).
- Re-running or widening PR test coverage from `pr-tests-and-coverage`.

## Decisions

1. **`{@html}` lint: eslint-disable with the AssistantChat rationale, not a rewrite.**
   - `renderMarkdown` already runs Marked → DOMPurify with an explicit allowlist (`web/src/lib/markdown.ts`). The rule flags every `{@html}` regardless.
   - Alternative considered: render plain text / a markdown component without `{@html}` — rejected for this fix; verdict prose already uses the shared helper and peers document the same disable.

2. **Go: `gofmt -w` on the three CI-named files only.**
   - No semantic edits. Prefer `gofmt -w` over hand-editing whitespace.

3. **Token ratchet: fix the UUID false positive in `presets.ts`; update baseline for `my/+layout.svelte`.**
   - Failure `presets.ts: 0 → 4` comes from the UUID regex (`-[0-9a-f]{…}` matches `ARBITRARY = /-\[[^\]]+\](?!:)/g`). Prefer rewriting the regex so the source no longer contains hyphen + character-class (e.g. build the pattern from a `hex` fragment via `RegExp` template) so the baseline stays at 0 for that file.
   - Failure `my/+layout.svelte: 2 → 3` is a real extra arbitrary (`transition-[…]` / `text-[10px]`). Follow the package contract: `pnpm check:tokens -- --update` (or the script’s documented update flag) and commit `web-token-baseline.json`. Do not loosen the ratchet to `>=`.
   - Alternative considered: only `--update` both files — rejected for `presets.ts` because it would permanently encode four false positives.

4. **Delivery: commit locally when asked; do not push.**
   - User constraint for this round. CI will stay red on the remote until a later push.

## Risks / Trade-offs

- **[Risk] eslint-disable is copy-paste debt** → Mitigation: identical comment wording to `AssistantChat.svelte` (“DOMPurify-sanitized markdown”) so reviewers recognise the pattern.
- **[Risk] UUID rewrite changes matching behaviour** → Mitigation: keep the same RFC-4122 hex shape and case-insensitive flag; add or extend a small unit assertion if one already covers `SESSION_ID`-style validation in that module.
- **[Risk] Baseline update without removing a avoidable arbitrary** → Mitigation: only update for the layout file; leave intentional `transition-[property]` / `text-[10px]` as recorded debt rather than inventing tokens in this hygiene pass.
- **[Risk] Local green, remote still red until push** → Accepted; document in tasks and do not push.

## Migration Plan

Not a deploy. After apply: local verify the three commands, commit on `feat/profile-contacts-experience-coverage` when requested, push only on explicit ask, then re-check PR #1737.
