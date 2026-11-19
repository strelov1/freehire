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
