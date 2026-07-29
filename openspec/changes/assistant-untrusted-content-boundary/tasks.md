## 1. Assistant markdown sanitizer

- [x] 1.1 Extract `renderMarkdown` from `AssistantChat.svelte` into a pure `web/src/lib/assistant/markdown.ts`, exporting the render function and keeping the scoped `openLinksInNewTab` hook (registered and removed around each call, with removal safe against a throw). Behaviour unchanged at this step.
- [x] 1.2 Add `web/src/lib/assistant/markdown.test.ts` covering the current permissive behaviour as a characterisation test, then flip it to the target: a markdown image and a raw `<img>` produce no `img` element.
- [x] 1.3 Replace the bare `DOMPurify.sanitize(html)` with the explicit policy — `ALLOWED_TAGS` for the prose markdown emits, `ALLOWED_ATTR` of `href`/`target`/`rel`, `ALLOWED_URI_REGEXP` pinned to `http`/`https`/`mailto` — mirroring `newDescriptionPolicy()` in `internal/sources/sanitize.go`.
- [x] 1.4 Extend the tests to the rest of the spec's scenarios: `<svg><use href>`, `<object>`, `<embed>`, `<iframe>`, `<form>`, `<video poster>` are all dropped; a `javascript:` and a `data:` link lose the URI; headings, lists, tables, emphasis, code blocks and `https` links survive with `target="_blank"` and `rel="noopener noreferrer"`.
- [x] 1.5 Point `AssistantChat.svelte` at the module for both sinks — the answer body and the thinking block — and confirm a partially streamed answer never renders the image (re-render on each update goes through the same policy).

## 2. Content-Security-Policy

- [x] 2.1 Add `img-src` (`'self'`, `data:`, `https://logo.freehire.me`) to `web/svelte.config.js`, and update the surrounding comment: it currently states images are unrestricted, and must instead record why `img-src` is pinned and why `connect-src` stays unset.

## 3. CV contact redaction in the service path

- [x] 3.1 Write the failing test first: the assistant's `cv_get` tool returns a document with no `full_name`, `email`, `phone` or `links` (`internal/handler/assistant_cv_tools_test.go`).
- [x] 3.2 Add the contact strip to `internal/cv` and expose it as a reader-shaped accessor (`Store.GetForModel`), with its own unit test that the returned record keeps the body and drops the four fields while the stored document is untouched.
- [x] 3.3 Point `cv_get` (`internal/handler/assistant_cv_tools.go`) at the accessor so the test from 3.1 passes.
- [x] 3.4 Route `GetCV`'s `auth.ViaAPIKey` branch (`internal/handler/cv.go`) through the same accessor, deleting the inline field-clearing so one implementation backs both readers. Existing CV handler tests must stay green.

## 4. Prompt

- [x] 4.1 Reword the `tailorPrompt` sentence in `internal/assistant/prompt.go` so it states what is enforced — the tools can neither read nor write the contact block — instead of implying general unreachability.

## 5. Verification

- [x] 5.1 `go build ./... && go vet ./... && go test ./...` and `pnpm test && pnpm lint && pnpm build` in `web/`.
- [x] 5.2 Verify the CSP against a real build (`vite build && vite preview`, not `vite dev`): load a job list, a company page, the tailor template gallery and a blog post, and confirm the console reports no CSP violation and company logos still render.
## 6. After deploy

Deferred by decision, not skipped: the assistant is gated to the beta group and a live turn
spends real inference, so this belongs on the deployed stack rather than a local one. The
rendering itself is covered pre-merge — 18 unit tests over real `marked` output, and the shipped
bundle was checked to carry the allowlist.

- [ ] 6.1 On prod, drive one assistant chat with a beta account and confirm ordinary formatted answers still render correctly (headings, lists, links, code), with no CSP violation in the console.
