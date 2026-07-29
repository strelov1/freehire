## Why

The in-app assistant feeds fully attacker-controlled text to the model and renders the model's
answer back into the DOM with `{@html}`, and neither end of that path is closed. A vacancy
description can carry instructions; the model can be talked into writing a markdown image; the
image URL can carry the candidate's name, email and phone, which the `cv_get` tool hands the model
in full. No click is required — the browser fetches the image the moment the node lands, mid-stream.

The read half of this is not a new gap but a regression: `cv-tailoring` already requires that the
tailoring agent never receives the CV contact block, and the redaction that implements it is keyed
on the HTTP transport (`auth.ViaAPIKey`). When the assistant moved in-process it stopped carrying a
key at all, so the guard silently stopped firing while its spec requirement stayed on the books.

Today only moderators and beta testers can reach the assistant (`requireRollout`). This closes
before the rollout opens, not after.

## What Changes

- The assistant's markdown renderer stops emitting request-triggering markup. Model output is
  sanitized with an explicit policy — no `img`/`picture`/`source`/`video`/`audio`/`iframe`/`form`/
  `svg`/`use`, and only `http`/`https`/`mailto` URI schemes — on both sinks it feeds (the answer and
  the thinking block). This mirrors the ban `internal/sources/sanitize.go` already applies to job
  description HTML for the same reason.
- The frontend CSP gains an `img-src` allowlist as the second layer, so an image that survives a
  future renderer regression still cannot leave the origin. `connect-src` is deliberately left
  unset — the existing note in `web/svelte.config.js` records that its absence is what keeps the
  Sentry ingest host, GA's collect endpoint, and the PostHog `/ingest` proxy reachable.
- The CV contact redaction moves out of the HTTP handler's transport check into a shared path both
  readers use, so it binds on *who is reading* rather than *how the request arrived*. The in-process
  `cv_get` tool starts obeying the requirement it already had.
- The tailoring system prompt stops claiming the candidate's contact details are "out of reach";
  once the redaction is real, the sentence describes the system instead of contradicting it.

## Capabilities

### New Capabilities
- `assistant-output-rendering`: how untrusted model output reaches the DOM — the sanitizer policy
  the assistant chat applies to markdown, and the CSP image allowlist backing it.

### Modified Capabilities
- `cv-tailoring`: the contact-block requirement is restated to bind on any read that puts the CV
  document in front of a model — the in-process tool path included — rather than only on reads made
  with the short-lived tailoring key.

## Impact

- `web/src/lib/assistant/AssistantChat.svelte` — `renderMarkdown` gains an explicit DOMPurify config.
- `web/svelte.config.js` — adds `img-src` (`'self'`, `data:`, `https://logo.freehire.me`). The only
  browser-side image sinks are `CompanyLogo.svelte` (the logo proxy) and same-origin
  `/cv-previews/*.svg`; OG cards render server-side and are not subject to browser CSP.
- `internal/cv` — gains the contact-redaction helper.
- `internal/handler/cv.go` (`GetCV`) and `internal/handler/assistant_cv_tools.go` (`cv_get`) — both
  route through it.
- `internal/assistant/prompt.go` — corrects the `tailorPrompt` claim about contact details.
- No migration, no API shape change: the redacted read already omits these fields for key callers,
  and the tool result is internal to a turn.
