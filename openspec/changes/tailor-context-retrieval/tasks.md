## 1. The posting reaches the agent as text

- [x] 1.1 Render the vacancy description to plain text and bound it in the agent's tailoring context, reusing the same helper `get_job` uses
- [x] 1.2 Test: a stored HTML description reaches the tool's result without tags, and an over-long one is truncated to the bound

## 2. The context carries the bank's answer and drops what the agent cannot act on

- [x] 2.1 Attach retrieved evidence (id, claim, `can_write_cv`) to every requirement the tool reports, bounded per requirement, using the same `experience.Store.Retrieve` scoring the search tool uses
- [x] 2.2 Report an empty list for a requirement the bank has nothing for, rather than omitting the field
- [x] 2.3 Degrade to no evidence (never an error) when the bank is unavailable — a tailoring context without retrieval is still worth reading
- [x] 2.5 Keep the dimension comments, strengths, gaps and recommendation out of the agent's context (the endpoint keeps serving them), with a test that says so
- [x] 2.4 Test: a requirement the bank evidences carries the id a later `cv_edit` can cite; a requirement it does not carries an empty list; no model is called

## 3. A rejected id names the valid ones

- [x] 3.1 When `experience_add` rejects an unknown `employment_id`, list the caller's employments with their ids in the error
- [x] 3.2 Test: the refusal names an existing role's id, and a retry using it succeeds within the same turn

## 4. The prompt spends rounds on edits

- [x] 4.1 Tell the tailoring agent to edit as each requirement is closed rather than researching everything first, and to read `cv_context`'s evidence instead of re-searching what it already carries
- [x] 4.2 Tell it not to restate the fit analysis — the candidate has it open beside the chat — and to keep its opening to what it is about to do
- [x] 4.3 Test the prompt states both, so a future edit cannot quietly drop them

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`, and the integration suites green
- [x] 5.2 Measured against the recorded baseline by field: `job` 4235 B, `missing_gap` 2297, `dimensions` 1756, `gaps`/`recommendation`/`strengths` 1273, `missing_have` 199. Rendering the posting saves ~700 B (not the ~7 KB first assumed — the description was 4.2 KB, not 11 KB); dropping the narrative sections from the agent's view saves ~3 KB more. A run against a live model is still unverified — no LLM credentials on this machine
