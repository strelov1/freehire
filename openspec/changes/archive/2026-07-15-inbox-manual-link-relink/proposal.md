## Why

In the mail inbox, an email can be auto-linked to an application, carry a
suggestion, or be unlinked. The linked state offers **Unlink**, and a suggestion
offers **Link / Not this** — but an email with **no link and no suggestion shows
nothing**: there is no way to link it to an application by hand. Worse, clicking
**Unlink** drops the email into exactly that dead state, so unlinking is a
one-way street with no way back.

The backend already supports manual linking (`POST /me/emails/:id/link` with a
slug → `link_source='manual'`) and the API client `api.linkEmail(id, slug)`
exists — it is simply never called from the UI. This change surfaces it.

## What Changes

- An unlinked email (no active suggestion) shows a **"Link to application"**
  picker listing the caller's tracked applications; choosing one manually links
  the email via the existing endpoint.
- Immediately after **Unlink**, the row offers **Undo**, which re-links the email
  to the application it was just unlinked from (remembered client-side).
- No backend change: the link/unlink endpoints and API client already exist.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `email-inbox`: add manual application-linking and post-unlink undo to the inbox
  reading pane's link controls.

## Impact

- `web/src/lib/components/InboxView.svelte`: orchestrate the new controls and hold
  the transient "last unlinked" state for Undo.
- `web/src/lib/components/ApplicationLinkPicker.svelte` (new): the searchable
  tracked-applications picker.
- Data source: existing `GET /me/tracking` (`api.listTracked`).
- No API, DB, or worker changes.
