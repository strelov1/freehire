## Context

The assistant is a bounded tool-calling loop in-process (`internal/assistant`), streamed over SSE
and gated to moderators and beta testers by `requireRollout`. Its tools return text nobody on our
side wrote: `search_jobs` hydrates full job descriptions, `cv_context` passes `job.Description`
verbatim, `read_current_page` returns whatever page the extension is standing on, and any signed-in
user can push a page of their own into the catalogue through `POST /jobs/resolve`, which imports a
JSON-LD `JobPosting` straight into the live index.

The chat renders the model's answer with `{@html}` after `DOMPurify.sanitize(html)` — called with
no configuration, so DOMPurify's permissive default allowlist applies and `<img>` survives. The
frontend CSP sets `script-src`, `base-uri` and `object-src` but no `img-src` and no `default-src`,
which by CSP's own rules leaves image loading unrestricted. Together those two facts make a
zero-click egress channel out of the chat transcript.

What that channel can carry is the other half. `cv_get` returns `rec.Document` whole, contact block
included. `cv-tailoring` already forbids exactly this, and `internal/handler/cv.go:244` implements
it — but behind `auth.ViaAPIKey(c)`, a test on the transport. The requirement was written when the
tailoring agent was an external process holding a scoped `cv` key; the in-process assistant holds
no key and issues no HTTP request, so the branch never runs for it.

Two useful precedents already exist in the repo. `internal/sources/sanitize.go` bans `<img>` from
job descriptions with an explicit prose allowlist, chosen over bluemonday's `UGCPolicy` precisely
because the content is third-party — and its comment names the tracking-pixel-on-every-viewer
scenario this change is about. And `requireEvidence` in `assistant_cv_tools.go` puts the tailoring
flow's one load-bearing rule in the service path rather than the system prompt, on the reasoning
that "a rule that lives only in a prompt is a rule a long conversation eventually loses."

## Goals / Non-Goals

**Goals:**

- No model output can cause the browser to issue a request to a host of the model's choosing.
- The CV contact block does not enter the model's context on any agent-facing read path.
- The redaction binds on the reader, not on the transport, so the next agent surface inherits it.
- The tailoring system prompt describes the system as it actually behaves.

**Non-Goals:**

- `connect-src` / `default-src`. Chosen deliberately (see Decisions); `img-src` closes the
  no-click channel, and the analytics/Sentry egress question deserves its own change.
- Review or quarantine for `POST /jobs/resolve`. Narrowing self-serve intake is a product decision
  about the contribution flow, not a rendering fix, and the boundary this change draws holds
  regardless of how much untrusted text reaches the model.
- Opening the rollout, or any change to `requireRollout`.
- Hardening `read_current_page` or the extension. It is an injection *source*; sources are assumed
  hostile by design and the defence belongs at the sink.
- Prompt-level instructions telling the model to ignore embedded instructions. Unenforceable, and
  the whole point is to stop relying on that.

## Decisions

### An explicit allowlist, not a list of forbidden tags

The obvious fix is `FORBID_TAGS: ['img', 'iframe', …]`. Rejected: a denylist over DOMPurify's
default allowlist has to stay ahead of every request-triggering element the HTML spec grows, and
the ones already shipping are easy to miss — `<svg><use href>`, `<object>`, `<embed>`, `<video
poster>` all fetch, and none appear on the obvious list.

Instead the renderer declares `ALLOWED_TAGS` covering exactly the prose markdown produces, and
`ALLOWED_ATTR` covering `href`/`target`/`rel`. This mirrors `newDescriptionPolicy()` in
`internal/sources/sanitize.go`, which made the same call for the same content class, so the two
untrusted-HTML sinks in the codebase now read alike. `ALLOWED_URI_REGEXP` pins schemes to
`http`/`https`/`mailto`, matching bluemonday's `AllowStandardURLs()` on the backend side.

Tags that markdown can emit but the policy drops: `input` (GFM task lists render as checkboxes —
a form control, and losing it costs a glyph) and `img`.

### `renderMarkdown` moves into a pure module

It currently sits inside `AssistantChat.svelte`, where no test can reach it. Moving it to
`web/src/lib/assistant/markdown.ts` makes the policy directly testable under the existing
`vitest.config.ts` (`environment: 'node'`, `src/**/*.test.ts`), which is how `deck.ts`, `chat.ts`
and `sse.ts` are already tested. `isomorphic-dompurify` is chosen for exactly this — it runs under
Node — so no jsdom environment switch is needed.

This also lets the sanitizer be tested against real `marked` output rather than hand-written HTML,
which matters: the interesting cases are the ones where markdown *syntax* becomes a tag.

### The DOMPurify hook stays scoped

`openLinksInNewTab` is registered and removed around each `sanitize` call because DOMPurify's hook
registry is global and shared with any other consumer. Keep that shape when moving the code; wrap
the removal so a throw inside `sanitize` cannot leave the hook installed.

### CSP: `img-src`, and nothing else

`img-src 'self' data: https://logo.freehire.me`.

Enumerated from the code rather than guessed: the only browser-side `<img>` sinks are
`CompanyLogo.svelte` (→ the `logo.freehire.me` proxy) and `TemplateGallery.svelte` (→ same-origin
`/cv-previews/*.svg`). OG cards are composed server-side and served as images, so they are never
subject to the browser's CSP on our own pages. Job descriptions carry no images at all — the ingest
sanitizer strips them — so nothing in the catalogue breaks. `data:` is included because Tailwind
emits `data:` SVG for a handful of utilities and inline marks.

`connect-src` stays unset, per the standing note in `web/svelte.config.js`: with no `default-src`,
its absence is what leaves the Sentry ingest host, GA's collect endpoint and the same-origin
PostHog `/ingest` proxy reachable. Adding it means enumerating all three correctly, and getting it
wrong fails silently — error reporting simply stops. That is a change worth making on its own, with
its own verification, not a rider on a security fix.

Note the residual: `img-src` does not stop an `<a href>` the user clicks, and does not stop
`connect-src`-class egress. The sanitizer is the primary control; CSP is the layer that survives a
sanitizer regression.

### Redaction becomes a reader-shaped accessor on the store

Options considered:

1. `Document.RedactContacts()` in `internal/cv`, called at both sites. One implementation, but
   still two places that must remember to call it — the failure mode we are fixing.
2. Redact inside `Store.Get` unconditionally and re-attach for the owner. Inverts the default
   safely but complicates every existing full-fidelity reader (PDF render, owner read, patch).
3. A second accessor, `Store.GetForModel(ctx, id, userID)`, that performs `Get` then strips the
   contact fields.

Taking (3). It names the distinction the requirement actually draws — *who is reading* — and makes
the safe path the one an agent surface reaches for by name. `cv_get` calls it unconditionally;
`GetCV` keeps its `auth.ViaAPIKey` branch but routes the redacting side through the same accessor,
so one implementation backs both and the HTTP behaviour is unchanged.

The stripped set matches what `GetCV` strips today: `FullName`, `Email`, `Phone`, and `Links` —
links are personal identifiers too (`github.com/<handle>`, `linkedin.com/in/<name>`), and the
existing handler already nils them even though the spec text only named the first three. The spec
delta records that.

### The prompt sentence is corrected, not deleted

`tailorPrompt` claims the candidate's "contact details are out of reach". After this change that is
true for reads as well as writes, so the sentence stays — but it is reworded to say what is
enforced (the tools cannot read or write them) rather than implying a general unreachability. The
prompt is documentation for the model, not a control; the controls are `GetForModel` and
`isContactHeaderField`.

## Risks / Trade-offs

- **`img-src` silently breaks an image sink nobody enumerated** → The sinks were read out of the
  source, not recalled. After deploy, check the browser console on a job list, a company page, the
  tailor gallery and a blog post for CSP violations. Rollback is a one-line config revert.
- **The allowlist strips markdown the assistant legitimately uses** → Tests run real `marked` output
  through the policy for every construct the prompt encourages (lists, tables, code, links).
  Failure mode is cosmetic and visible, not silent.
- **`GetForModel` is added but a future surface calls plain `Get`** → Mitigated but not eliminated;
  the accessor makes the right call obvious and the spec now states the rule in reader terms. A
  test asserts `cv_get` returns no contact field, so the regression has a tripwire.
- **Removing `Links` narrows what the agent can reason about** → It already could not see them on
  the HTTP path, so no capability is lost relative to the specified behaviour.
- **CSP is not enforced in dev the way it is in prod** → SvelteKit emits the header in both, but
  `vite dev` differs enough that verification belongs against a real `vite build && vite preview`.

## Migration Plan

No data migration, no API shape change. `cv_get`'s result loses three fields the spec says it
should never have carried; nothing persists or reads them downstream. Deploy is a normal release —
frontend rebuild plus backend. Rollback is a revert; no state is written.

## Open Questions

None blocking. `connect-src` and the `/jobs/resolve` intake review are deferred by decision, not by
uncertainty, and are recorded as Non-Goals.
