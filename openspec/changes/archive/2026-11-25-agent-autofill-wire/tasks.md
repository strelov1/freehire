## 1. hire transport (relay)

- [x] 1.1 Add a JWT-authenticated WebSocket relay in hire: authenticate the connection from the session JWT; register the connection under its user (`sub`); forward `{id,tool,args}` / `{id,result}` frames between that user's harness and extension ends. Owner-scoped — never bridge across users.
- [x] 1.2 Reject unauthenticated connections; return an error result (never hang) when a call's counterpart end is absent.
- [x] 1.3 Tests: authed round-trip forwards a call+result; cross-user isolation; unauthenticated rejected; missing-counterpart returns an error result.

## 2. extension browser-tool executor

- [x] 2.1 Wire client (side panel / background): connect to the hire relay with the stored JWT, reconnect on drop, dispatch inbound `{id,tool,args}` to handlers and return `{id,result}`.
- [x] 2.2 `read_form` primitive: reuse `form.ts` `extractForm`; return fields with label / type / value / `combo` flag, iframe-aware; unit-test the label extraction + combo flag.
- [x] 2.3 `fill_simple` primitive: address by **label**, match + write atomically (text/textarea/checkbox/native-select) via `applyFills` native-event dispatch; **skip** custom-widget comboboxes (report deferred); unit-test label matching, no-drift, and combobox-skip.

## 3. agent driver (map + fill)

- [x] 3.1 Fetch the signed-in user's profile (reuse hire `/me/autofill-profile`); the agent builds a `{label → value}` plan for standard fields, leaving unmapped fields out (no fabricated values).
- [x] 3.2 Drive the turn: `read_form` → plan → `fill_simple` over the wire; report what was filled and which comboboxes are not-yet-supported.

## 4. panel wiring

- [x] 4.1 Route the panel's Autofill action through the agent path (read_form → plan → fill_simple), keeping the deterministic autofill as a fallback until the agent path is proven.
- [x] 4.2 Surface the result (filled fields; comboboxes reported as not-yet-fillable) and errors.

## 5. Verify

- [x] 5.1 Extension: `vitest run` + `svelte-check` + `wxt build`; hire: relay tests green.
- [x] 5.2 Live end-to-end: on a real apply form, the agent fills the standard fields through the wire; comboboxes are reported, not corrupted.
  - Run against a real hire server: `POST /me/autofill/run` → agent → relay → the
    extension's own executor → DOM. Filled name/email/phone/city/LinkedIn;
    reported the react-select combobox as deferred without touching it; dropped
    an invented cover-letter answer before it reached the browser. The model was
    a local stand-in returning a fixed plan — everything else was real.
  - Then repeated in a real Chrome with the unpacked extension loaded, driving
    the panel's own Autofill button — first on a local page whose form sits in an
    iframe (proving the frame fan-out), then on two live Greenhouse postings
    (Stripe, Scout Motors). Standard fields filled; every react-select widget
    reported as deferred and left untouched; nothing submitted.
  - Three defects the synthetic runs had hidden, now fixed with tests:
    CSS-hidden controls (a recaptcha textarea, collapsed sections) were being
    read as fields; the report named controls with no label, which cannot be
    addressed; and ATS forms mark a field required with `aria-required`, which
    the observation ignored. The report now leads with required fields and the
    panel names a few and counts the rest.
  - Known limit, for the widget slice: on Stripe the "left for you" list is led
    by 30 country checkboxes — options of one multi-select question that the
    wire sees as 30 separate controls. Grouping them needs the widget model the
    next change introduces; guessing at it here would be the "smart filler" the
    design rejects.
