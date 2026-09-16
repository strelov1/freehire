## 1. Manifest

- [x] 1.1 Add `debugger` and `downloads` permissions to `wxt.config.ts`'s manifest block.

## 2. Tailored-CV resolution (`lib/freehire.ts`)

- [x] 2.1 Add a pure function that picks the matching tailored CV (if any) for a job slug
      out of the `GET /me/cvs` list response (reuse `cvTailoredResponse`'s shape:
      `id`/`job_slug`), with a test covering: a match present, no match, and multiple tailored
      CVs where only one matches the slug.
- [x] 2.2 Add `getTailoredCVForJob(slug, token)` wiring that calls `GET /me/cvs` and applies
      2.1's picker.
- [x] 2.3 Add `getCVPdfBytes(id, token)` (or reuse an existing fetch helper) to pull the
      rendered PDF from `GET /me/cvs/:id/pdf` as bytes.

## 3. Frame-scoped element resolution

- [x] 3.1 Add a pure function that, given `extractUploads`'s reported `{form, frame}` for the
      target upload field, produces the same selector/frame-addressing expression
      `fill_simple` already uses, so the CDP step and the ordinary fill path share one way of
      naming "which control." Test it against the existing `fill-simple-call.json`-style
      fixtures for frame/form addressing.

## 4. Download-to-path helper (background)

- [x] 4.1 Add a background helper that base64-encodes PDF bytes into a `data:` URL, calls
      `chrome.downloads.download` into a `freehire-tailored-cv/` subfolder with
      `conflictAction: 'uniquify'`, and resolves the absolute path once
      `chrome.downloads.onChanged` reports `state.current === 'complete'`, bounded by a
      timeout that reports a "check your Downloads prompt" error (Risks in design.md).
- [x] 4.2 Test the promise/timeout/error-classification logic with a faked
      `chrome.downloads` (this is glue over a browser API per `extension/AGENTS.md`'s "test
      the logic, not the transport" — keep the test to the state machine, not real downloads).

## 5. CDP attach/write/detach (background)

- [x] 5.1 Add a background handler that, given a tab id, a resolved file path (from §4) and
      the frame-scoped expression (from §3): `chrome.debugger.attach` → `Runtime.evaluate`
      (scoped to the target frame) to get the input's `objectId` → `DOM.setFileInputFiles` →
      `chrome.debugger.detach` in a `finally`, on every exit path.
- [x] 5.2 Classify and surface the two named failure modes distinctly: tab already open in
      DevTools (attach fails before any banner) and cross-origin embedded frame (the resolved
      frame is not reachable from the top frame's execution context) — both from Risks in
      design.md.
- [x] 5.3 Wire the panel-to-background message (extending `protocol.ts`'s `RuntimeMessage`)
      that carries the job slug and triggers §2-§5 end to end, returning success/failure to
      the panel.

## 6. Panel UI (`MatchCard.svelte`)

- [ ] 6.1 Show the "Attach tailored CV" action only when both hold: a tailored CV exists for
      the job in view (§2) and `extractUploads` reports a file field on the current page —
      matching the spec's three scenarios (shown, hidden on no tailored CV, hidden on no
      upload field).
- [ ] 6.2 Wire the click to the message from §5.3; show its success/failure result inline
      (reuse the panel's existing inline-error pattern rather than introducing a new one).

## 7. Verification

- [ ] 7.1 `npm test` (vitest) green for all new pure/unit-testable pieces.
- [ ] 7.2 `npm run build` + load unpacked; live-verify against a real Greenhouse posting with
      a tailored CV: action appears, click attaches the file, field shows it, debugger
      detaches (banner disappears) whether the call succeeds or is made to fail.
- [ ] 7.3 Live-verify the two failure modes from §5.2 report a clear message rather than
      hanging or silently doing nothing.
