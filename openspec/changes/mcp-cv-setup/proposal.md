## Why

A person who connects freehire in Claude or ChatGPT before uploading a CV cannot get set up
from the chat. The signed-in MCP server has `get_profile` and `update_profile`, but no way to
give freehire a CV and no answer to "what is still missing for this account" — so the agent
sends the person to the website, which is where most of them stop. AIApply's MCP shows the
shape that works: one status read naming the single next step, a CV accepted as text for a
first setup, and preferences suggested from it that the agent confirms before saving.

## What Changes

- **`add_cv`** (signed-in MCP server, OAuth grant only): takes the CV as text (≤ 30,000
  characters) for an account that has none yet, and runs it through the website's own CV
  upload use case — the stored document, the free background extraction (the stored-CV parse,
  `plan.WithFreeAI`) and the experience-bank import. Returns the profile values the CV
  suggests (specializations, seniorities, skills, in `update_profile`'s field names); it saves
  nothing to the profile. Refused when the account already has a CV.
- **`get_setup_status`** (OAuth grant only, read-only): reports, from existing state, whether
  the account has a CV (and whether reading it finished), job-search preferences, an active
  alert, a paid plan and the agents-may-apply switch, plus ONE `next_step` naming the tool to
  call or the page to send the person to.
- The upload use case (store + derive) is extracted from the two HTTP handlers into one method
  the handlers and the tool share.
- Account server instructions name `get_setup_status`; the `set_up_my_profile` prompt walks the
  new flow when the tools are served.

## Non-goals

- Replacing an existing CV over MCP, or accepting a file upload over MCP.
- A save tool for preferences: `update_profile` already saves confirmed values.
- New tables or columns.

## Impact

- `internal/api/handler`: new `assistant_cv_add_tool.go`, `assistant_setup_status_tool.go`;
  `resume.go` (shared store-and-derive); one line in `mcpapp_account.go`.
- `internal/api/mcpapp`: `account.go` instructions, `account_prompts.go`, AGENTS.md.
