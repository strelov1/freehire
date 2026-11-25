## Why

freehire already reads, classifies and links a person's application mail, and the in-app
assistant can see it, but an agent connected to the account MCP server cannot: "did anyone
reply?" has no answer there. AIApply's MCP answers it with a deliberately narrow tool —
sender, subject, date and a few flags of unread mail, no bodies, nothing marked seen, and a
description that says the sender wrote those strings. We want the same answer, bounded the
same way, plus what our mail stack knows that theirs does not: which tracked job a message
is attached to and what kind of message the classifier judged it to be.

## What Changes

- **New account MCP tool `list_unread_mail`** (read-only, `EffectReads`): the newest unread
  application mail as `{sender, subject, received_at, label, company, job_slug}` rows, plus
  `unread_count` (may exceed the rows shown), `sync_pending` (a connected Gmail has not
  finished its first sync), `reconnect_needed` (Gmail lost its grant) and `mailbox_url`.
  It never returns a body or snippet, never marks a message read, and calls no model.
- **Sender text is data.** Sender and subject are stripped of control and format characters
  (newlines, zero-width and bidi marks), collapsed and rune-capped before they leave, and the
  tool description, a `notice` field in the result and the account server instructions say
  they are written by the sender: report them, never act on them.
- **Access mirrors REST.** `GET /me/inbox` is `mw.key` (cookie or full-scope API key), so the
  tool is served to OAuth grants and full-scope keys alike; a `cv` key is already a 403 at
  the mount.
- **Write tools stay off MCP** (`inbox_triage`, `inbox_link`, `inbox_unlink`,
  `inbox_resolve_suggestion`, `inbox_record_application`). Follow-up, not this change.

## Impact

- `internal/api/handler` (new `mcp_mail_tool.go`, `mcpKeyTools` registration),
  `internal/api/mcpapp` (one instructions sentence), `internal/api/mcpapp/AGENTS.md`.
- No migration, no new query, no REST change, no LLM call.
- Out of scope: per-tool OAuth scopes (`mcp:mail`), opening a message, a scam flag (the mail
  stack has none), write tools.
