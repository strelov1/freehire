## 1. Pending-action transport

- [x] 1.1 Write failing tests in `web/src/lib/recentAuth.test.ts` for the draft: `begin…` with a
  draft writes `freehire.reauth.draft` beside the verifier; `begin…` without one writes no draft
  key; `consumeReauthDraft()` returns the draft and removes it; a second call returns null; a
  malformed stored value returns null rather than throwing.
- [x] 1.2 Add the optional third `draft` argument to `beginProviderReauthentication` and export
  `consumeReauthDraft` in `web/src/lib/recentAuth.ts`, using `sessionStorage` beside the existing
  verifier/return keys (design D4).
- [x] 1.3 Write a failing test asserting the draft is cleared when the reauth attempt fails, so an
  expired exchange leaves nothing behind for a later visit to reopen; make it pass.

## 2. Shared confirmation surface

- [x] 2.1 Write failing component tests for `ConfirmIdentity`: an account with `has_password`
  renders a password input and no provider buttons; an account without renders one button per
  **active** provider and no password input; a `revocation_pending` provider is not offered; the
  identities request is made only for a passwordless account.
- [x] 2.2 Write failing component tests for the confirmed state: after a successful round trip the
  component reports identity confirmed with a countdown, and dropping the hint restores the
  confirmation controls (design D5).
- [x] 2.3 Implement `web/src/lib/components/ConfirmIdentity.svelte` plus its message catalogue
  (en + ru), exposing the password value to its caller rather than submitting it (design D2).
- [x] 2.4 Write a failing test asserting the component never renders a "confirmation required"
  statement with no control by which to provide it, for both account shapes.

## 3. API keys — creation

- [x] 3.1 Write failing tests for `CreateApiKeyDialog`: the dialog carries name, expiry and the
  confirmation block; a password account's `reauthenticatePassword` is called immediately before
  `createApiKey`; a `428` from either call re-presents the confirmation instead of a generic error.
- [x] 3.2 Write a failing test that opening the provider confirmation stores `{name, days}` as the
  draft, and that mounting with that draft present reopens the dialog with the fields restored and
  the action **not** performed (spec: returning does not perform the action).
- [x] 3.3 Implement `web/src/lib/components/CreateApiKeyDialog.svelte` and move the create form out
  of `ApiKeysView.svelte` behind a "Create key" button.
- [x] 3.4 Update `ApiKeysView.messages.ts` (en + ru) for the new surface and delete the strings the
  old inline form no longer uses.

## 4. API keys — revocation

- [x] 4.1 Write a failing test reproducing the dead end: with a password account, opening the
  revoke dialog and confirming must be completable using only controls inside the dialog.
- [x] 4.2 Pass `ConfirmIdentity` as `ConfirmDialog`'s `children` snippet in `ApiKeysView.svelte` and
  read the password from it, so revoke no longer depends on the create form's state.
- [x] 4.3 Write a failing test that a `428` during revoke re-presents the confirmation inside the
  dialog and leaves the key in the list; make it pass.

## 5. Account deletion

- [x] 5.1 Write failing tests re-asserting the existing guarantees before touching the component:
  the delete action stays disabled until the typed email matches, and a successful deletion clears
  session state and redirects.
- [x] 5.2 Write a failing test that leaving for a provider stores `{reopen: true}` and **no** typed
  email, and that on return the dialog is open, states identity was confirmed, and the delete
  action is disabled until the email is typed again (spec: the typed address is re-entered).
- [x] 5.3 Replace `DeleteAccountButton.svelte`'s inline confirmation with `ConfirmIdentity` and add
  the draft round trip; keep its own warning copy and typed-email barrier unchanged.
- [x] 5.4 Reconcile `DeleteAccountButton.messages.ts` (en + ru) with the strings that moved into the
  shared component, removing the ones it no longer owns.

## 6. Password change

- [x] 6.1 Write a failing test that a `428` from the password-change flow presents the confirmation
  step rather than the generic error (`my/security/+page.svelte`'s `keyFor` currently maps only 401
  and 400).
- [x] 6.2 Map `428` on that surface to a message naming the real remedy (sign out and back in),
  NOT to a `ConfirmIdentity` block — see design D7 — and confirm no draft is written for this
  flow (design D4).

## 7. API reference

- [x] 7.1 Remove the "API keys" group from `web/src/lib/docs/api-spec.ts` and add the paragraph to
  the existing "What is not here" overview section explaining why key management cannot be scripted
  and linking to `/my/api-keys` (design D6).
- [x] 7.2 Run `pnpm run gen:openapi` in `web/` and commit the regenerated
  `web/static/api-reference.openapi.json`; confirm no `api-keys` tag or path survives in it.
- [x] 7.3 Regenerate `docs/API.md` and confirm the same three endpoints are gone from it.
- [x] 7.4 Run `npx @redocly/cli@2.49.0 lint web/static/api-reference.openapi.json --extends=minimal`
  and `pnpm check:links`, since the new paragraph adds a link and the removed group had an anchor.

## 8. Verification

- [x] 8.1 Run `pnpm test`, `pnpm lint` and `pnpm check` in `web/`; run `pnpm check:dead` for the
  exports the deleted form no longer uses.
- [x] 8.2 Verify by hand against an armed gate: create a key as a provider-only account
  including the full round trip. Deferred to PRODUCTION rather than a local run — the trip needs
  a real OAuth client and a real `hire_recent_auth` cookie, which a local backend cannot issue
  for the provider path, the very leg that was broken.

  **It found one.** The round trip died at the provider: `ProviderV2` asked for a return to
  `/api/v2/auth/oauth/<name>/callback` while only `/api/v1/...` was ever registered, and GitHub
  refused outright ("The redirect_uri is not associated with this application"). Only Google had
  both listed, and nothing in the repository said the two paths existed or had to be configured
  twice. Fixed separately (#2884): both flows now build the same registered callback, a test
  pins them together, and they are told apart at the callback by which state cookie returns.
  Confirmed working in production afterwards, sign-in included.

  Worth recording as the reason this task existed: every other check in this change passed
  against a flow that could not complete. A test suite cannot see a URL a third party has never
  been told about.
- [x] 8.3 Confirm on the running docs page that `/docs/api` no longer offers a curl for
  `POST /me/api-keys` and that the replacement paragraph's link resolves.
