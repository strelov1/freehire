## 1. Tool

- [x] 1.1 Unit tests (RED): shape, no body/snippet/id, cap, unread-only query, header sanitizing,
      company only on a confirmed link, served to key and grant sets, effect classification.
- [x] 1.2 Implement `list_unread_mail` (`internal/api/handler/mcp_mail_tool.go`) and register it in
      `mcpKeyTools`.

## 2. Instructions

- [x] 2.1 Test + one sentence in `accountInstructions` when the tool is served; stay within the
      length budget.

## 3. Integration

- [x] 3.1 Integration test over the account endpoint: grant and key see the tool; injected
      subject returned as data; other user's mail invisible; nothing marked read; sync_pending.

## 4. Docs + ship

- [x] 4.1 `internal/api/mcpapp/AGENTS.md`: the tool, its gate, the write-tool follow-up.
- [x] 4.2 Verification suite, PR.
