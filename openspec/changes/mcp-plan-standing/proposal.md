## Why

An agent connected to the signed-in MCP server cannot tell what the account may still pay
for. `get_profile` carries no plan, so the agent learns about limits only from a 402 after
it has already called `auto_apply_start` or a tailoring tool — and, on a free account, it
may retry what the paid gate refuses every time. AIApply's MCP has a `get_credits_summary`
tool for exactly this; freehire already holds the data (`internal/ai/plan`, served over REST
as `GET /me/plan`) but offers it to no agent.

## What Changes

- New read-only account MCP tool **`get_plan`**: the tier (`free`/`pro`/`ultra`), when the
  paid entitlement ends (if it does), when today's allowances reset, and for every metered
  feature today's limit, used, remaining, unlimited and enforced flags, plus the URL of the
  site's plan page.
- It reads the same use case as `GET /me/plan` (`loadPlan`), so the two doors cannot
  disagree. It carries no prices, invoices or payment source: billing is the website's.
- Its description says the free plan includes no AI features, so an agent tells the person
  to upgrade rather than retrying.
- Served to OAuth grants and full-scope API keys alike (`mcpKeyTools`): `GET /me/plan` is
  open to a key over REST.
- `remaining` is given only for an enforced limit: a shadow limit refuses no paid plan, so a
  "0 remaining" there would be a false stop sign. `paid_until` is the end of the current paid
  period, which an auto-renewing subscription renews.
- The description is phrased around auto-apply, the one metered feature an MCP tool spends,
  and states the charging rule: a new posting uses one; retrying, reviewing or restarting
  the same posting uses nothing.
- The account server's instructions gain one short line, only for a session that is served
  auto-apply: call `get_plan` before auto-apply.

## Non-goals

- Not added to the in-app assistant (its support agent already has `my_account`, which
  carries prices and history and is not fit for a third-party model).
- No change to the HTTP contract.

## Impact

- `internal/api/handler/mcp_plan_tool.go` (new), `mcpapp_account.go` (registration).
- `internal/api/mcpapp/account.go` (instructions).
