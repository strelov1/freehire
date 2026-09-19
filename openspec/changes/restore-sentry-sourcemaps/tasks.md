## 1. The credential check

- [x] 1.1 RED: add `web/scripts/sentry-credential-check.test.mjs` pinning the verdict
      function's answers — no credential configured is a stated no-op, a credential Sentry
      accepts passes, a credential Sentry rejects fails and names the rejection. Cover the
      under-scoped case from design's Risks (403), the wrong-slug case (404), the
      half-configured case, and every "cannot tell" answer.
- [x] 1.2 GREEN: add `web/scripts/sentry-credential-check.mjs` — a pure verdict function
      over the probe's outcome plus a thin `main()` that performs the probe and exits
      0/1. Name it beside the existing `*-smoke.mjs` scripts and follow their shape.
- [x] 1.3 Probe `GET /api/0/organizations/<org>/chunk-upload/` — the call `sentry-cli`
      itself makes first, which is the closest question to the upload's own that a
      side-effect-free read can ask. **Not proven to reject an under-scoped token**: that
      is what 5.4 measures, and until it runs this is intent, not property. (The first
      draft probed the project's release list, which Sentry also grants to a read-only
      `project:read` token; an under-scoped credential would have passed. Found in
      review.) A second read
      of the project's own releases catches a mistyped `SENTRY_PROJECT`, which the
      organisation-scoped first call cannot see.
- [x] 1.4 REFACTOR + `simplify` pass over the new script and test; re-run the tests.
- [x] 1.5 Review the diff (`requesting-code-review`); fix Critical + Important.

## 2. The release path

- [x] 2.1 Call the check from `freehire-ops' scripts/host2/release.sh` in the existing Sentry block,
      BEFORE web's `pnpm run build`, so a bad credential costs seconds rather than the
      full web build.
- [x] 2.2 Make the existing claim true: it printed "source maps will be uploaded to Sentry"
      unconditionally whenever the env file was readable. It now prints that only after the
      credential is accepted, names the org/project it was accepted for, and prints the
      minified-traces notice when no credential is configured.
- [x] 2.3 Failure message names source-map upload as the cause and states that the live
      color is untouched — the wording the `assets < 100` and `facet smoke` gates beside it
      already use.
- [x] 2.4 Treat ONLY exit 1 as a verdict. `node` missing (127) or the script absent from a
      checkout that has not pulled this commit both mean the check did not run, which is not
      evidence about the credential — reporting them as a rejection would make this gate a
      single point of failure for every deploy. Found in review.
- [x] 2.5 Run the check as `freehire`, like everything else this script executes out of the
      checkout, and pass `SENTRY_URL` through to the build as well as the check so the two
      cannot disagree about which Sentry they mean. Found in review.
- [x] 2.6 `shellcheck freehire-ops' scripts/host2/release.sh` clean (the `artifacts` CI job covers every
      tracked `*.sh`).

## 3. The prose that was wrong

- [x] 3.1 Correct `web/vite.config.ts`'s source-maps comment: it described only the ABSENT
      token. It now states that an invalid token is swallowed by `@sentry/sveltekit`'s
      `vite/sourceMaps.js` bare catch, so `errorHandler` cannot make it fatal and the
      build's exit status carries no answer — and points at the release check that does.
- [x] 3.2 Same correction in the two documents CLAUDE.md points a reader at for this
      subject: `internal/platform/observability/AGENTS.md` and `web/AGENTS.md`. Both still
      said only "build succeeds without them". Found in review.
- [x] 3.3 `eslint` clean on the touched files.

## 4. Rotate the credential on the host

**Order is load-bearing: 4.1-4.3 before 4.4.** Copying `release.sh` to the host is what arms
the check, and arming it against the dead token refuses every release — including everyone
else's. Until 4.4 runs, this whole change is inert on production, which is why merging it
first was safe.

- [ ] 4.1 Mint a replacement Sentry token with the scope the upload needs. **User action** —
      the agent cannot mint one.
- [ ] 4.2 Install it in `/opt/freehire/env/sentry-build.env` (0600 root), replacing the
      dead value. Prefer setting `SENTRY_URL` explicitly there: `sntrys_`-prefixed org
      tokens carry their own region, and a cross-region redirect would drop the
      `Authorization` header and surface as a 401.
- [ ] 4.3 **Revoke the old token.** It was read out of `journald`, so this is a rotation,
      not a swap — see design's Risks.
- [ ] 4.4 Copy the changed `release.sh` to the host: `deploy/` does not deploy itself
      (`freehire-ops' provision/host2/AGENTS.md`). Confirm with ``freehire-ops`' scripts/host2/drift-check.sh` exiting 0.

## 5. Verify it actually worked

- [ ] 5.1 Run a release and confirm the check passes and the build proceeds.
- [ ] 5.2 Confirm Sentry now shows artifacts for that release — the measurement that
      failed before this change was 0 of 100 releases with any files, and an empty
      `artifact-bundles` list.
- [ ] 5.3 Open a production issue in Sentry and confirm the frames name our own files
      rather than `fn=t` / `fn=f`. This is the outcome the change exists for; the steps
      above are only the means.
- [ ] 5.4 **Negative control for the scope claim:** mint a second token scoped `project:read`
      only and confirm the check REJECTS it. This is the only thing that turns design's
      stated mitigation from a claim into a measurement — and the first draft of the probe
      would have failed it. Raised in review.
- [ ] 5.5 Negative control for the opt-out: unset the credential on a throwaway build and
      confirm the release still succeeds and states that traces will be minified.
- [ ] 5.6 Settle design's Open Question with the now-available positive control: record
      which API surface shows a successful debug-id upload, and decide whether the check
      should be tightened from credential to artifact (do it, or note the seam).

## 6. Close out

- [ ] 6.1 `verification-before-completion` over the whole change.
- [ ] 6.2 `finishing-a-development-branch` — PR, CI, merge.
- [ ] 6.3 `/opsx:archive` then `/opsx:sync`.
