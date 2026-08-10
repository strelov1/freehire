## 1. Backend wire shape and sanitization

- [x] 1.1 Add `Signal` type (`Quote`, `Insight` string fields) and `HiddenSignals []Signal` field on `Analysis` in `internal/matchanalysis/matchanalysis.go`
- [x] 1.2 Add `maxSignals`, `maxSignalQuoteRunes`, `maxSignalInsightRunes` bound constants alongside the existing `max*` constants
- [x] 1.3 Add `sanitizeSignals` function: drop entries with a blank `quote` or `insight`, truncate each to its bound, cap the count at `maxSignals`
- [x] 1.4 Thread `HiddenSignals` through `buildAnalysis` (new parameter, nil-safe default to `[]Signal{}` matching the existing `reqs`/`strengths`/`gaps` nil-handling pattern)

## 2. Stage 1 extraction

- [x] 2.1 Add `HiddenSignals []Signal` to `stage1Out` in `internal/matchanalysis/analyzer.go`
- [x] 2.2 Extend `stage1SystemPrompt()` with the hidden-signals instruction block: return `"hidden_signals"` (array, max 5) with `"quote"` (verbatim JD excerpt) and `"insight"` (one line on pace/ownership/team-stage/culture implication); explicit instruction that an empty array is correct for a generic posting, never force an entry
- [x] 2.3 In `AnalyzeStream`, call `sanitizeSignals` on `s1.HiddenSignals` alongside the existing `sanitizeRequirements(s1.Requirements)` call, and pass the result into `buildAnalysis`
- [x] 2.4 Extend the `Event` struct and the `EventRequirements` emission with the sanitized `HiddenSignals`, so the SSE payload carries both `Requirements` and `HiddenSignals` at the same point in the stream

## 3. Backend tests

- [x] 3.1 Unit tests for `sanitizeSignals`: drops blank quote, drops blank insight, truncates over-length fields, caps count at 5 (mirror the existing `sanitizeRequirements`/`cleanList` test style)
- [x] 3.2 `prompt_test.go`: assert `stage1SystemPrompt()` contains the new `hidden_signals` instruction
- [x] 3.3 `analyzer_test.go`: assert Stage 1's `HiddenSignals` output threads into the final `Analysis.HiddenSignals`, and that a Stage-1 response omitting the key yields an empty (not nil) slice on the served `Analysis`

## 4. Contract + frontend wiring

- [x] 4.1 Run `cmd/gen-contracts` to regenerate the TS wire types for the new `Analysis.HiddenSignals`/`Signal` shape
- [x] 4.2 Update `reduceMatchEvent` in `web/src/lib/matchAnalysis.ts` to carry `hidden_signals` through from the Stage-1 (`requirements`) event and the final analysis
- [x] 4.3 Add a "Hidden Signals" section to the match analysis page (`web/src/routes/match/[slug]/`), rendered directly after the Requirement Match table and before Strengths/Gaps; each entry shows the quote and its insight; the section renders nothing when `hidden_signals` is empty

## 5. Verification

- [x] 5.1 `go build ./...`, `go vet ./...`, `go test ./...`
- [x] 5.2 `go vet -tags=integration ./...`
- [x] 5.3 Manually exercise the match analysis page against a real posting with distinctive wording (e.g. "fast-paced", "high ownership") and confirm the Hidden Signals section renders; confirm it is absent for a bland/short posting
