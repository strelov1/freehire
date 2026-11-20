# internal/api/mcpapp: unlisted tools

## Unlisted tools: `find_tools` and `call_tool`

`deferred.go`. Account tools with under 50 calls in 30 days (`deferredTools`, plus every
`buddy_*`; measured from `freehire_mcp_tool_calls_total` on 2026-11-18) are still registered but
left out of `tools/list`: the OAuth session lists 35 tools instead of 56, ~15K schema tokens
instead of ~21.5K. `find_tools` returns them with their schemas, best matches to a query first;
`call_tool` runs one. A receiving middleware does both: it filters `tools/list` and rewrites
`call_tool {name, arguments}` into a `tools/call` of the target, so the target's own handler,
logs and metrics run, and nothing but a served deferred tool can be reached that way. It must
be added after `ObserveServer` (later middleware runs outside). A direct call by name still
works and must keep working: freehire-cli and the npm freehire-mcp call `unsubscribe` and
`update_subscription` so. `call_tool` is annotated for the worst tool it runs (destructive).
Never defer a tool the instructions or a prompt name (`TestNoNamedToolIsDeferred`).

The experience-bank and tailored-CV tools (`experience_*`, `cv_*`;
`internal/api/handler/mcp_cv_tools.go`) were added unlisted from the start, so the listed set
did not grow. They are the npm freehire-mcp package's `/me/experience` and `/me/cvs` tools,
served to grants and full-scope keys (every route behind them takes a key). Each runs the
route's own contract handler under `auth.AgentContext`, so an MCP agent is a keyed caller:
redacted `cv_get`, `cv_edit` as the agent behind the evidence wall, `UpdateAtom` keeping
provenance, `DeleteEmployment` refusing to cascade. `cv_tailor` first passes the paid-AI gate
its route sits behind (`requirePaidAI`), then the handler's own allowance check, so this door
is not a way around paying. `cv_render` renders and returns a `pdf_url` the signed-in person
opens, not the bytes. Nothing here replaces the person's source CV: that stays a website act.
