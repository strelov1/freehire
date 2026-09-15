## Context

`recentauth` is a step-up gate: `POST /me/password`, `POST /me/api-keys`,
`DELETE /me/api-keys/:id` and `DELETE /me` require, on top of the session cookie, a
short-lived proof bound to `(user_id, token_version, session_hash)` and held in the HttpOnly
cookie `hire_recent_auth`. Without it they answer `428 recent_auth_required`
(`internal/api/handler/auth.go:123-135`, `:233-238`). The gate is armed only when
`AUTH_V2_ENABLED` is true — on in production since 2026-08-12, off locally and in CI, which is
why client-side gaps here are invisible to `go test`, to `pnpm test`, and to a developer
running the app.

A proof is issued by exactly two routes: `POST /api/v2/auth/reauth/password`, and the OAuth
reauth exchange that `/my/reauth` performs. Whether a member can use the first depends on
`has_password`, which an OAuth-only account does not have.

Three client surfaces trigger gated actions today, each with its own copy of the same logic:

| Surface | State of the copy |
|---|---|
| `ApiKeysView.svelte` | Create form holds the password input; the revoke `ConfirmDialog` demands a password but renders no input and is modal — the action cannot be completed. The provider buttons appear only after a `428`, and leaving for the provider discards the typed key name. |
| `DeleteAccountButton.svelte` | Correct in shape — password and provider buttons both live inside the dialog — but leaving for the provider closes the dialog and discards the typed email, and the return lands on `/my/security` with no indication anything succeeded. |
| `my/security/+page.svelte` | `keyFor()` maps 401 and 400; `428` falls through to a generic "something went wrong". |

`ConfirmDialog` already accepts a `children` snippet (`design-system/src/confirm-dialog.svelte:17`)
which `ApiKeysView` does not pass — the dead end is an omission, not a missing capability.

On the documentation side, `web/src/lib/docs/api-spec.ts` is the single source of truth for both
the rendered `/docs/api` (Scalar over `web/static/api-reference.openapi.json`) and the generated
`docs/API.md`. Its "API keys" group (line 996) publishes `curl … -b cookies.txt` recipes that
answer `428` in production for every reader.

## Goals / Non-Goals

**Goals:**

- One confirmation surface, used by all three gated screens, so the behaviour cannot drift
  again.
- The revoke path becomes completable at all.
- A provider round trip preserves the pending action and reports its outcome.
- `428` is handled on every surface that can receive it.
- The API reference stops publishing a recipe that cannot work, and points at the surface that
  can.

**Non-Goals:**

- The server gate: the 428, the proof's TTL and binding, `AUTH_V2_ENABLED`, and the two
  issuing routes are unchanged.
- Native/mobile reauth (`mobileauth` platforms `ios`/`android`).
- The legacy no-JTI session that can never bind a proof (`auth.ErrNoSessionID`). This change
  makes that state legible — the member is told confirmation is needed and offered a control —
  but resolving it still means signing in again. Detecting and forcing a session rotation is
  separate work.
- Reading the proof's true remaining lifetime. The cookie is HttpOnly by design.

## Decisions

### D1 — One `ConfirmIdentity` component, owned by the confirmation, not by the caller

A single Svelte component renders the whole confirmation block: it resolves `has_password`,
fetches connected identities when needed, renders either the password input or one button per
active provider, and renders the confirmed state. Callers pass what action is pending and get
back the proof-obtaining behaviour.

*Alternative considered:* leave three copies and fix each in place. Rejected — the three copies
are precisely what produced three different behaviours from one requirement, and a fourth gated
action would start a fourth copy. The spec's "every gated action offers a way to obtain the
proof" needs one place that can be true.

*Consequence:* `DeleteAccountButton` loses its inline copy. That is a rewrite of working code,
accepted because the shared component must be proven against the surface that already had it
right.

### D2 — `ConfirmIdentity` produces the proof; the caller decides what to do about failure

The caller calls `await identity.prove()` immediately before its own request. `prove()` spends
a held proof as-is, or buys one with the typed password. It swallows nothing: a refused
password throws, and the caller words that for its own surface.

*Revised during the simplify pass.* The original decision put `api.reauthenticatePassword()`
at each call site, so that it stayed adjacent to the action it authorized. All three sites
then wrote the same conditional — `if (!identity?.isConfirmed()) await
api.reauthenticatePassword(password)` — and three copies of one decision is exactly what
produced three different behaviours from one requirement before this component existed. The
call is still adjacent to the action; it is just no longer restated three times.

`isConfirmed()` remains exposed, because the caller still needs it for one thing `prove()`
cannot decide: whether a `401` should read as "wrong password" (it must not, to a member who
never typed one).

*Alternative considered:* have `ConfirmIdentity` obtain the proof on its own "Confirm" button,
turning every action into two steps. Rejected — for a password account the proof is
instantaneous, so a separate step is ceremony; and splitting it opens a window in which the
proof is spent but the action is not attempted.

### D3 — Show the confirmation up front, not after a 428

`DeleteAccountButton` deliberately reveals provider buttons only after the server refuses,
because leaving for the provider destroyed the dialog: offering the trip before it was needed
would have cost the member their typed input for nothing. D4 removes that cost, so the reason
expires and the confirmation becomes a visible part of the action.

The `428` path remains, as the correction when the client's belief is wrong (D5).

### D4 — A pending action is carried in `sessionStorage`, beside the PKCE verifier

`beginProviderReauthentication(provider, returnTo, draft?)` gains a third argument, stored under
`freehire.reauth.draft` next to the existing `verifier` and `return` keys. `/my/reauth` already
routes back by `returnTo`; the destination surface consumes the draft on mount and reopens
itself.

What each caller stores is a deliberate choice, not a generic form dump:

| Caller | Draft | Rationale |
|---|---|---|
| Create API key | `{name, days}` | Ordinary input; losing it is the whole complaint. |
| Delete account | `{reopen: true}` | Reopen the dialog; the typed email is an anti-mistake barrier and is re-typed. |
| Change password | nothing | A password is never written to storage. |

`sessionStorage` (not `localStorage`) because the round trip is same-tab and a draft must not
outlive the tab. The draft is removed when consumed, so an unrelated later visit does not
reopen a dialog.

*Alternative considered:* carry the draft through the OAuth `state` parameter. Rejected — the
server would then hold client form state it has no business knowing, and `state` is already
bound to the attempt.

### D5 — The confirmed state is a client-side hint; the server is the authority

The client cannot read `hire_recent_auth`, but it does not have to guess when the proof dies:
**both issuing routes already answer with `recent_auth_expires_at`**, and `api.ts` already
returns that timestamp from `reauthenticatePassword` and `exchangeOAuthReauthentication` — every
caller today discards it. On a successful exchange the expiry is stored beside the other reauth
keys and the surface shows "identity confirmed" counting down to it.

*Revised during implementation.* This decision originally proposed recording the local moment
and counting down the deployed 10-minute default, treating the countdown as deliberately
optimistic. Reading `api.ts` showed the server's own expiry was already on the wire, so the
countdown is a measurement rather than an estimate, and nothing needs to know `RECENT_AUTH_TTL`.

Only the PROVIDER path records it. `reauthenticatePassword` returns the same field and it is
still discarded, deliberately: under D2 a password is re-typed and spent per action, so a
recorded expiry would buy that path nothing and would only add a second way for the stored
hint to disagree with reality. `recentAuth.ts` exports no setter, so there is no path by
which a caller can record one without saying why.

The expiry is still not permission. A proof can die before its stated expiry — the session's
`token_version` can be bumped, the proof consumed, the cookie cleared — so a `428` still
overrules the hint, drops it, and re-presents the confirmation step. `logout()` clears it too:
the hint is per-tab, and signing in as somebody else would otherwise inherit it.

*Alternative considered:* an endpoint reporting whether a live proof exists. Rejected as
unnecessary: the gated call itself already answers that question, and a second endpoint would be
a second truth that can disagree.

### D6 — API keys leave the reference entirely, rather than being marked unusable

The "API keys" group is removed from `api-spec.ts` and a paragraph is added to the existing
"What is not here" overview section (line 139), which already exists for endpoints that are
"deliberately left out because calling them directly is meaningless".

*Alternative considered:* keep the entries and add a warning. Rejected — the entries render a
copyable curl button, and a copyable command that cannot succeed is the defect being fixed.

*Alternative considered:* keep the group with an empty endpoint list. Rejected — it would emit
an OpenAPI tag with no operations, which `@redocly/cli lint` flags in CI.

The generated artifact must be refreshed with `pnpm run gen:openapi` in `web/`; the `docs` CI
job diffs `web/static/api-reference.openapi.json` against the generator and fails on staleness.

### D7 — The password-change surface answers 428 with the remedy, not with another password box

*Added during implementation.* The plan said this surface would handle `428` "via
`ConfirmIdentity`". Building it showed that would be nonsense here: the form is rendered only
for an account that HAS a password, and the current-password field already IS the
confirmation — `reauthenticatePassword` runs on it immediately before the change. Reaching a
`428` therefore means the server accepted the password and the proof still would not bind,
which is the legacy no-JTI session (`auth.ErrNoSessionID`). A second password box beside the
one just accepted would ask the member to repeat what already worked.

So this surface maps `428` to its own message naming the only thing that actually fixes it:
sign out and sign back in. The scenario "Every gated surface handles 428" was amended to say
so in as many words — "presents the confirmation step, **or** — where the confirmation the
member could offer has already been given and refused — names what would actually resolve
it" — rather than leaving the code quietly at odds with a document that said otherwise.

This is the one gated surface that does not mount `ConfirmIdentity`, and the reason is
structural rather than an omission.

## Risks / Trade-offs

- **The gate is off locally, so a broken client passes every local check.** This is exactly how
  issue #2191 shipped account deletion broken for everyone for 13 days. → Component tests drive
  `ConfirmIdentity` and each caller against a stubbed API returning `428`, so the behaviour is
  asserted without the flag. Manual verification runs with
  `AUTH_V2_ENABLED=true MOBILE_AUTH_CALLBACKS=web=http://localhost:5173/my/reauth`.
- **Rewriting `DeleteAccountButton` touches the one irreversible action on the site.** → Its
  existing guarantees are re-asserted as tests before the rewrite: the action stays disabled
  until the typed email matches, and the typed email is never restored after a round trip.
- **Showing provider buttons up front (D3) makes a `connectedIdentities()` call for accounts
  that have no password.** → One request, only for the accounts that need it — but "only when
  a dialog opens" does not come for free. `design-system/src/dialog.svelte` renders its
  `children` unconditionally, so a `ConfirmIdentity` placed in a dialog mounts on page load
  and fires that request whether or not the dialog is ever opened (and, when a proof is held,
  ticks its countdown for the life of the page). Callers therefore wrap it in
  `{#if open}…{/if}` inside the dialog, and the component's header comment says so.
- **A draft left in `sessionStorage` by an abandoned round trip could reopen a dialog
  unexpectedly.** → Consumed-and-removed on read, and scoped to the tab.
- **Removing the documented endpoints changes a published reference.** → The removed recipe
  never worked; the replacement paragraph names the surface that does and links to it.
- **`pnpm run gen:openapi` not being run turns a docs edit into a red CI.** → It is an explicit
  task, not a step folded into another.

## Migration Plan

Frontend-only; no schema, no API, no worker. Ships in one deploy. Rollback is a revert — no
state is written anywhere that would outlive it, since the only new storage is per-tab
`sessionStorage`.

## Open Questions

None. The countdown reads the server's own `recent_auth_expires_at` (D5), so reconfiguring
`RECENT_AUTH_TTL` needs no client change.
