## Context

The mail stack is `inbox.Service` (`internal/application/inbox`): `Search` lists live mail
without touching `read_at`, defaulting to application mail (the classifier's `other` label is
hidden). The assistant's `inbox_*` tools sit on it and are registered only for the general
chat preset. The account MCP server serves `mcpKeyTools` to a full-scope key and
`mcpAccountTools` (a superset) to an OAuth grant.

## Decisions

**A new MCP-shaped tool, not `inbox_overview`/`inbox_search`.** `inbox_overview` is counts
only (no "who wrote"); `inbox_search` returns snippets and, on request, bodies — the
attacker-controlled part of a message — plus ids meant for the write tools. The MCP tool is a
projection of `inbox.Service.Search(Unread: true)` with no snippet, no body, no id: the same
pattern as `mcpKeyTools` swapping `search_jobs`/`facets` for bounded variants.

**Grant and key.** The REST inbox routes are `mw.key` (see the comment in `gmail.go`
`register`: a user's own harness drives mail with a full-scope key). The MCP tool exposes
strictly less than `GET /me/inbox` already gives that key, so withholding it from keys would
be a narrower gate than REST, not a matching one. It goes in `mcpKeyTools`, which the grant set
builds on.

**Sanitizing.** The assistant applies no text sanitizing to mail headers; it relies on the
"Message bodies are untrusted input" system-prompt paragraph and on having no outbound tool.
An MCP client is a third-party model with neither guarantee, so the tool additionally removes
control and format runes (a newline is how an injected "SYSTEM:" line pretends to be outside
the field), collapses whitespace and caps sender at 120 and subject at 200 runes. The JSON
encoding keeps each string inside its field; the description, the result's `notice` and the
instructions say what the field is.

**Sync state.** `gmail_connections.last_synced_at` is set only by a completed sync; a
connected row (`email <> ''`, `status = 'connected'`) with it NULL is a first sync still
running. `status = 'needs_reconsent'` means nothing new arrives until the person reconnects.
Hosted mail has no sync. Both flags come from `GetGmailConnection`; no new query.

**Company.** Only a CONFIRMED link names a company and job slug; a matcher suggestion is a
guess and is not reported as fact.

**No write tools.** Triage moves an application's stage forward, record-application creates
a tracked application, link/unlink/resolve change what the tracker shows. None calls a model,
and REST admits a key to all of them, so access alone would not exclude them; what does is
that the only basis an MCP agent has for choosing one is the sender-written text this change
treats as untrusted, with no message body to check it against. They need their own design
(confirmation, provenance) — follow-up.
