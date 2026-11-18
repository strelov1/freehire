## Why

Candidates already searching on Freehire spend too much time reading listings. Let them delegate shortlisting under their existing filters and refine the selection conversationally.

## What Changes

- Open the existing assistant panel from the standalone jobs list.
- Persist a validated search snapshot and session-local selection brief.
- Share search execution with the listing, enforce the captured constraints, and show at most five explained recommendations.
- Resume the same conversation after closing or reloading; explicitly start a new one to use changed page filters.

## Capabilities

### New Capabilities

- `search-shortlist-assistant`: Search-bound shortlisting, feedback, and resumption.

### Modified Capabilities

None. Existing unbound chat and public search retain their behavior.

## Impact

Assistant session schema and sqlc queries; handler search assembly and scoped tools; jobs-list panel host, session API, and localization. No new runtime dependencies or employer submissions.
