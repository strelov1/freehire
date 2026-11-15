## Why

Candidates cannot currently choose which confirmed achievements a cover letter uses before
paying for generation. Persisting that choice makes drafting understandable and consistent
between the workspace and assistant.

## What Changes

- Add owner-scoped, revisioned evidence selection for a vacancy, with automatic and explicit modes.
- Enforce saved selections before charging and across all candidate achievement prompt inputs.
- Show optional evidence review, validation failures and changed-evidence draft freshness.
- Preserve existing automatic drafting and stored letters; defer CV and screening integration.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cover-letter-draft`: Candidate-controlled evidence, persistence, generation enforcement and freshness.

## Impact

Coverletter domain/store, a new PostgreSQL migration and sqlc queries, shared API/assistant drafter,
CV cover-letter routes/contracts, and the existing Svelte Tailor cover-letter tab. No new model
calls or dependencies are needed for evidence review.
