## Why

Three account surfaces are gated behind a short-lived proof of recent credential control
(`recentauth`, HTTP 428) that the client cannot always obtain, and on one of them the user is
asked for a password there is nowhere to type. A member reported being unable to create an API
key at all. The gate itself is right — an API key is a permanent bearer credential and a
hijacked session must not mint one — but it is currently reachable only by guessing.

The same confusion reaches integrators: the public API reference documents
`POST/GET/DELETE /me/api-keys` with a copyable `curl … -b cookies.txt` example that cannot
work. Those endpoints are cookie-only **and** recent-auth-gated, so a scripted call returns
428 no matter what the reader does. Readers took the documented recipe at face value and got
stuck.

Nothing in `openspec/specs/` describes the recent-auth gate at all — not the server contract,
not the client's obligation to be able to satisfy it. The behaviour exists only in code
comments, which is why three independent copies of the same client logic drifted apart without
anything noticing.

## What Changes

- **Introduce one client contract for identity confirmation.** A single surface pattern —
  password field for an account that has one, provider buttons for an account that does not —
  used by every gated action, replacing three divergent copies.
- **Fix the API-keys revoke dead end.** The confirmation input moves inside the dialog that
  needs it. Today the revoke dialog demands a password whose only input lives in the create
  form behind the modal backdrop, so the action cannot be completed at all.
- **Survive the provider round trip.** Confirming through an OAuth provider is a full-page
  navigation that today destroys the pending action. The pending action is preserved across it
  and resumed on return, and the returning user is told the confirmation succeeded.
- **Show the price of admission up front.** The confirmation step is presented as a step of the
  action, not as a red error raised after the server refuses.
- **Handle 428 where it is currently unhandled.** The password-change surface maps 428 onto a
  generic "something went wrong", which is a silent dead end for a session that cannot bind a
  proof.
- **Remove the three unusable endpoints from the API reference** and state in the existing
  "What is not here" section that keys are managed in the account area, with a link. **BREAKING**
  for anyone who had copied the documented curl — but that recipe never worked.

Out of scope: the server-side gate, the proof's TTL and binding, the 428 status itself, and the
mobile/native auth flows. None of them change.

## Capabilities

### New Capabilities

- `identity-confirmation`: the step-up confirmation contract shared by every action gated on
  recent credential control — which actions are gated, what a client must offer so the gate can
  actually be satisfied, how a pending action survives the provider round trip, and what the
  user is told at each step.

### Modified Capabilities

- `api-documentation`: "Documented API coverage" currently requires the reference to cover API
  keys as endpoints. It is amended to exclude endpoints that no API client can call, and to
  require the reference to name the surface that performs the action instead.
- `account-deletion`: "Deletion surface states the consequences" gains the requirement that the
  deletion surface can actually obtain the confirmation it needs, including across the provider
  round trip.

## Impact

Frontend only; no Go, no SQL, no migration, no API change.

- `web/src/lib/components/ApiKeysView.svelte` — create form becomes a dialog; revoke dialog
  gains its own confirmation
- `web/src/lib/components/DeleteAccountButton.svelte` — adopts the shared confirmation
- `web/src/routes/my/security/+page.svelte` — handles 428
- `web/src/lib/recentAuth.ts` — carries a pending action across the provider round trip
- `web/src/routes/my/reauth/+page.svelte` — return path
- new shared confirmation component, plus message catalogues (en + ru) for each surface
- `web/src/lib/docs/api-spec.ts` — the single source of truth for both `/docs/api` and
  `docs/API.md`; `web/static/api-reference.openapi.json` is regenerated from it by
  `pnpm run gen:openapi` and diffed by CI
