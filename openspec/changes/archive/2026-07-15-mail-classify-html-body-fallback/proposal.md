## Why

The mail classifier that maps an inbox email onto an application stage reads only
the plain-text body (`emails.body_text`). Many ATS senders (Gem, Ashby,
Greenhouse) send **HTML-only** mail with no `text/plain` part, so `body_text` is
empty and the classifier sees only the sender name and subject. A real rejection
("Regarding your Application to Fingerprint" → "we regret to inform you… not to
proceed") then gets classified as `screening`, because the bare subject reads
like a recruiter reaching out. Users see wrong application stages.

The full message text is already stored in `emails.body_html`; it just never
reaches the classifier. The fix is small and adds no new dependency
(`github.com/jaytaylor/html2text` is already an indirect dep via `enmime`).

## What Changes

- The classify-mail worker feeds the classifier the email's **readable body**:
  the plain-text part when it has real content, otherwise the HTML part stripped
  to text. The reading pane still renders the rich HTML unchanged.
- `ClaimEmailClassificationBatch` also returns `emails.body_html`; the worker's
  `Claimed` carries it; a new pure `readableBody(text, html)` selects the source.
- No change to the classification prompt, the controlled vocabulary, the stage
  rules, or stored-data semantics (`body_text`/`body_html` keep their meanings).

## Capabilities

### New Capabilities
- `email-body-classification`: how the readable body is selected from an email's
  plain-text and HTML parts before it is classified.

### Modified Capabilities
<!-- none: the parent email-application-linking capability is not yet in main specs -->

## Impact

- `internal/db/queries/mail_classification.sql` (+ regenerated sqlc): claim query
  returns `body_html`.
- `internal/maillink/`: `Claimed.BodyHTML`, new `readableBody` helper, runner
  wiring.
- `cmd/classify-mail/store.go`: map `body_html` into `Claimed`.
- Dependency `github.com/jaytaylor/html2text` promoted from indirect to direct.
- Operational (out of scope here): re-classifying already-misclassified emails
  requires re-enqueueing them (reset `classified_at`); noted, not done in code.
