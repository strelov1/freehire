## 1. Tool

- [x] 1.1 `get_plan` tool over `loadPlan`: tier, paid-until, resets-at, per-feature
      limit/used/remaining/unlimited/enforced, plan page URL; unit tests for the shape
      (remaining, unlimited, no payment fields).
- [x] 1.2 Register in `mcpKeyTools`; classify its effect in `TestMCPAccountTools_DeclareTheirEffect`.

## 2. Instructions

- [x] 2.1 One line in `accountInstructions` when `get_plan` is served; instruction tests
      (presence, length budget).

## 3. Integration

- [x] 3.1 Integration test: a pro account with usage and a free account read `get_plan` over
      the account server; the result agrees with `loadPlan`.
