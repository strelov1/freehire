## ADDED Requirements

### Requirement: Agents Can Read the Account's Plan Before Metered Work
The signed-in MCP server SHALL offer a read-only `get_plan` tool, to OAuth grants and
full-scope API keys alike, reporting the account's tier, when its paid entitlement ends,
when today's allowances reset, and for every metered AI feature today's limit, use,
remaining amount, and whether the limit is enforced. It SHALL read the same use case as
`GET /me/plan` and SHALL carry no prices, invoices or payment source, pointing at the
website's plan page instead.

#### Scenario: A paid account reads its allowances
- **WHEN** a pro account that has used part of today's auto-apply allowance calls `get_plan`
- **THEN** the result names the `pro` plan, its end date, and an `auto-apply` allowance whose used, limit and remaining agree with `GET /me/plan`

#### Scenario: A free account is told it has no AI
- **WHEN** a free account calls `get_plan`
- **THEN** the result names the `free` plan with nothing remaining on any metered feature and carries the plan page URL to upgrade from

#### Scenario: No payment details reach the agent
- **WHEN** any account calls `get_plan`
- **THEN** the result carries no price, payment source or invoice
