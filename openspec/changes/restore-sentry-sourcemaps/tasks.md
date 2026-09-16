## 1. The credential check

- [ ] 1.1 RED: add `web/scripts/sentry-credential-check.test.mjs` pinning the verdict
      function's three answers — no credential configured is a stated no-op, a credential
      Sentry accepts passes, a credential Sentry rejects fails and names the rejection.
      Cover the under-scoped case from design's Risks: an HTTP 403 on the scope the upload
      needs must fail, not pass.
- [ ] 1.2 GREEN: add `web/scripts/sentry-credential-check.mjs` — a pure verdict function
      over the probe's outcome plus a thin `main()` that performs the probe and exits
      0/1. Name it beside the existing `*-smoke.mjs` scripts and follow their shape.
- [ ] 1.3 Probe with a call that needs the SAME scope the source-map upload needs (release
      write), not a bare org read — a token that can read the org and not write releases
      must fail the check, per design.
- [ ] 1.4 REFACTOR + `simplify` pass over the new script and test; re-run the tests.
- [ ] 1.5 Review the diff (`requesting-code-review`); fix Critical + Important.

## 2. The release path

- [ ] 2.1 Call the check from `deploy/bin/release.sh` in the existing Sentry block
      (around line 131), BEFORE `pnpm run build`, so a bad credential costs seconds
      rather than the full web build.
- [ ] 2.2 Make line 137's existing claim true: it currently prints "source maps will be
      uploaded to Sentry" unconditionally whenever the env file is readable. It must print
      that only after the credential is accepted, and must print the minified-traces
      warning when no credential is configured.
- [ ] 2.3 Failure message names source-map upload as the cause and states that the live
      color is untouched — the wording the `assets < 100` and `facet smoke` gates beside it
      already use.
- [ ] 2.4 `shellcheck deploy/bin/release.sh` clean (the `artifacts` CI job covers every
      tracked `*.sh`).

## 3. The comment that was wrong

- [ ] 3.1 Correct `web/vite.config.ts`'s source-maps comment: it describes only the ABSENT
      token. State that an invalid token is swallowed by `@sentry/sveltekit`'s
      `vite/sourceMaps.js` bare catch, so `errorHandler` cannot make it fatal and the
      build's exit status carries no answer — and point at the release check that does.
- [ ] 3.2 `eslint` clean on the touched file.

## 4. Rotate the credential on the host

- [ ] 4.1 Mint a replacement Sentry token with the scope the upload needs. **User action** —
      the agent cannot mint one.
- [ ] 4.2 Install it in `/opt/freehire/env/sentry-build.env` (0600 root), replacing the
      dead value.
- [ ] 4.3 **Revoke the old token.** It was read out of `journald`, so this is a rotation,
      not a swap — see design's Risks.
- [ ] 4.4 Copy the changed `release.sh` to the host: `deploy/` does not deploy itself
      (`deploy/AGENTS.md`). Confirm with `./deploy/check-drift.sh` exiting 0.

## 5. Verify it actually worked

- [ ] 5.1 Run a release and confirm the check passes and the build proceeds.
- [ ] 5.2 Confirm Sentry now shows artifacts for that release — the measurement that
      failed before this change was 0 of 100 releases with any files, and an empty
      `artifact-bundles` list.
- [ ] 5.3 Open a production issue in Sentry and confirm the frames name our own files
      rather than `fn=t` / `fn=f`. This is the outcome the change exists for; the steps
      above are only the means.
- [ ] 5.4 Settle design's Open Question with the now-available positive control: record
      which API surface shows a successful debug-id upload, and decide whether the check
      should be tightened from credential to artifact (do it, or note the seam).
- [ ] 5.5 Negative control: unset the credential on a throwaway build and confirm the
      release still succeeds and states that traces will be minified.

## 6. Close out

- [ ] 6.1 `verification-before-completion` over the whole change.
- [ ] 6.2 `finishing-a-development-branch` — PR, CI, merge.
- [ ] 6.3 `/opsx:archive` then `/opsx:sync`.
