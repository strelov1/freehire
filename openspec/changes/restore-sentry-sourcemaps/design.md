## Context

The frontend build is supposed to upload source maps to Sentry so that a minified
production stack trace can be read. It does not, and has not for as long as there is data
to look at.

What is measured, not inferred:

- The host's `sentry-cli` is answered `Invalid token (http status: 401)` during upload —
  logged by `freehire-autodeploy` at 2026-09-16T04:30:20Z.
- Of the **100** most recent `freehire-web` releases in Sentry (2026-09-08 → 2026-09-16),
  **none** has a non-null `fileCount`, and the project's `artifact-bundles` list is empty.
- The build exits 0 anyway, and the release proceeds to completion.

The last line is the interesting one, and it is not an accident. `@sentry/sveltekit`'s own
`vite/sourceMaps.js` wraps the upload in a bare catch:

```js
try {
  if (typeof originalWriteBundle === "function") {
    await originalWriteBundle({ dir: outDir });
  }
} catch {
  console.warn("[Source Maps Plugin] Failed to upload source maps!");
  ...
}
```

So the wrapper swallows the failure **above** `@sentry/vite-plugin`'s `errorHandler`
option, whose documented default is to throw and stop the bundle. Configuring the plugin
cannot make this fatal; the swallow is not ours to remove.

`web/vite.config.ts` meanwhile documents only the absent-token case ("inert without
`SENTRY_AUTH_TOKEN` — the build still succeeds"), which is true and is not the case
production is in.

## Goals / Non-Goals

**Goals:**

- A release configured to upload source maps either uploads them or fails.
- A release not configured to upload them succeeds and says so.
- The host carries a credential Sentry accepts.

**Non-Goals:**

- Changing anything about what is reported or when. The error paths, the DSN gating and
  the transient-noise filter are untouched.
- Re-uploading source maps for past releases. The traces already in Sentry stay minified;
  nothing can be done about them and the quota would not pay for it.
- Raising the Sentry plan or changing quota. Separate concern.
- Fixing the credential's storage shape (it currently reaches the build through a `sudo`
  command line and is therefore in `journald` in clear text). Real, but a different
  change — noted under Risks.

## Decisions

### Verify the credential, not the artifact

The check asks Sentry whether the configured token is accepted, before the build starts.
It does **not** ask whether the release ended up with artifacts.

*Why not the artifact, which is what we actually care about?* Because we cannot currently
tell what a successful upload looks like on the API. Debug-id uploads create artifact
bundles rather than release files, and with zero successful uploads in the entire
observable window there is no positive control to calibrate against — a check written
against a guessed field would pass or fail for reasons nobody could confirm, which is the
failure this change exists to end, reintroduced one layer up.

*Alternative considered — run `sentry-cli sourcemaps upload` ourselves* and take its exit
status as the evidence. Rejected: the plugin also injects debug IDs and flattens the SSR
maps, so a second uploader would either duplicate that work or upload maps the bundle does
not reference. We would be replacing a silent gap with a subtle one.

*Cost of the narrower check, stated plainly:* a valid token whose upload nevertheless
produces nothing still passes. That is the residue. It is smaller than what is being
fixed, and once one successful upload exists there is finally a positive control — at
which point the check can be upgraded from credential to artifact. The seam is named here
rather than built now.

### Fail the release, not the build

The verification lives in the release path, not inside `vite.config.ts`, because the
bundler's exit status structurally cannot carry this answer (see the bare catch above). It
runs **before** the build rather than after, so a misconfiguration costs seconds instead
of the three minutes the web build takes.

Failing is deliberate rather than warning. A token that somebody typed is a claim that
readable traces are wanted; a release that quietly drops them is not the release that was
asked for. The blast radius is bounded by the existing blue/green discipline — a release
that fails before the color switch leaves production exactly where it was, which is what
`migrations FAILED — not touching green or the live color` already does on the same path.
The escape hatch is to remove the whole `sentry-build.env` file, which is the explicit
opt-out below. Removing only the TOKEN is not the escape hatch and never was: it leaves
`SENTRY_ORG` and `SENTRY_PROJECT` behind, which is the half-configured case this same
design requires to fail. All three go or none do.

### An absent token stays a stated no-op

No credential means no upload, a successful release, and one line saying this release's
traces will be minified. This keeps "off on purpose" and "broken" from reading alike — the
distinction the current comment asserts but nothing enforces — and keeps every local and
CI build working with no Sentry credential at all.

### The comment in `vite.config.ts` is part of the change

It currently describes the case production is not in. Prose about code is tested by
nothing, so it is corrected in the same change that makes the behaviour true, and it
names the wrapper's swallow so the next reader does not spend an afternoon rediscovering
why `errorHandler` did not help.

## Risks / Trade-offs

- **A dead token now blocks deploys entirely** → That is the intent, and it is bounded:
  the failure is pre-build and pre-color-switch, the message names the cause, and removing
  `sentry-build.env` ships immediately without one.
- **The check passes a token that is valid but under-scoped** (it can read, but cannot
  upload) → Probe `GET /organizations/{org}/chunk-upload/`, the call `sentry-cli` itself
  makes before uploading anything. **This mitigation is not yet measured.** The first draft
  probed the project's release list, which Sentry also grants to a read-only `project:read`
  token — review caught it — and review of the replacement notes that chunk-upload's own
  scope map may likewise admit `org:read` beside the write scope, since the upload is the
  `POST` and this is the `GET`. Probing the upload's own first call is the best available
  answer without a positive control; what settles it is task 5.4, a deliberately
  under-scoped token that the check must REJECT. Until that runs, treat "an under-scoped
  token is caught" as intended rather than demonstrated.
- **The residue: a valid token whose upload still yields nothing** → Not covered, named
  above, and revisitable as soon as a positive control exists.
- **The credential is visible in `journald`** → `release.sh` reads it from a 0600 root file
  and passes it with `--preserve-env`, whose comment says this "keeps it out of `ps(1)`".
  That is true and insufficient: sudo logs the preserved environment in its own journal
  line, token value included, which is where this one was read from. Out of scope here, but
  it makes replacing the token a **rotation, not a repair** — the old value must be revoked,
  not merely swapped.
- **`deploy/` does not deploy itself** (`deploy/AGENTS.md`) → Copying the changed release
  path to the host is a task in this change, not a follow-up, and `./deploy/check-drift.sh`
  is the confirmation.

## Migration Plan

**The credential is replaced BEFORE the check reaches the host, not after.** An earlier
draft had it the other way round — land the change against the dead token, let every deploy
fail loudly, and call that "the moment the problem becomes visible". That was rejected, and
it was wrong twice over. It would have bought visibility nobody needed (the problem was
already measured, in this document) at the price of blocking every unrelated deploy until a
human happened to be free to mint a token. And it misread its own mechanism: merging this
change does NOT arm the check. `deploy/` does not deploy itself (`deploy/AGENTS.md`), so the
host keeps running its own copy of `release.sh` until someone copies the new one over. That
copy is the arming step, and it is step 3.

1. Merge the change. Nothing on the host changes: the check is in the repository and the
   host still runs its old `release.sh`, so releases behave exactly as before. This step is
   inert by design, which is what makes the ordering below free to choose.
2. Mint a replacement token with the scope the upload needs, install it in
   `/opt/freehire/env/sentry-build.env`, and **revoke the old one** — it was read out of
   `journald`, so this is a rotation, not a swap.
3. Copy the new `release.sh` to `/opt/freehire/bin/`. This is what arms the check, and it
   is safe now precisely because step 2 already happened. Confirm with
   `./deploy/check-drift.sh` exiting 0.
4. Confirm the next release passes the check and that Sentry shows artifacts for it.
5. Re-read a production issue and confirm the frames name our own files.

**Rollback:** remove `/opt/freehire/env/sentry-build.env` on the host — the whole file, not
just the token. Releases resume immediately, source maps go back to being absent, and the
release says so on every run — the pre-change behaviour, minus the silence.

An earlier draft of this paragraph said "unset `SENTRY_AUTH_TOKEN`", which would have done
the opposite: `SENTRY_ORG` and `SENTRY_PROJECT` would still be set, that is the
half-configured case, and this design requires the release to FAIL on it. The rollback
would have tightened the gate it was meant to open — in the document somebody reads while
the gate is already blocking them. `release.sh`'s own message was right throughout ("fix
it, or **remove it** to release without source maps"); this paragraph was the copy that
drifted, which is the hazard this whole change is about.

## Open Questions

- Which API surface shows a successful debug-id upload for a release — `fileCount`,
  `artifact-bundles`, or neither? Unanswerable until one upload succeeds; step 3 of the
  migration is the opportunity to settle it and to decide whether the check should then be
  tightened from credential to artifact.
- Does the same gap exist for any other Sentry-uploading surface (the extension, the
  design system)? Not investigated; neither is known to upload today.
