## 1. Parser (`web/src/lib/searchOperators.ts`)

- [x] 1.1 `parseOperators(text)` → `{ text, title[], company[], source[] }` with the grammar
  in design.md (quotes, unclosed quote, unknown keys, empty values, case-insensitive keys).
- [x] 1.2 `activeOperator(text, caret)` → the operator token under the caret
  (`{ key, value, start, end }`) or null.
- [x] 1.3 `replaceToken(text, token, key, value)` → text with the token replaced by
  `key:value ` (quoted when the value has a space) and the new caret position.

## 2. Plan building

- [x] 2.1 `operatorPlan(parsed, resolvers)` → an `ApplyPlan`; `title:` folds free text into one
  quoted title-only `q`. `unresolvedToken(text, resolvers)` names the first value that
  resolves to nothing, which stops the commit.
- [x] 2.2 Source matching against the source distribution (key or `sourceLabel`).
- [x] 2.3 Company matching against companies-endpoint items (exact name or slug).

## 3. HeaderSearch wiring

- [x] 3.1 Enter: parse; with no operators run today's path unchanged; otherwise resolve,
  apply the plan; an unresolved value stops the commit with the caret on it.
- [x] 3.2 Contextual dropdown mode from `activeOperator` at the caret; company/source/title
  rows; pick replaces the token and keeps focus.
- [x] 3.3 Strip operators from the text the ordinary fetches send.

## 4. Verify

- [x] 4.1 `pnpm test` (vitest), `pnpm check`, eslint on changed files.
- [x] 4.2 Drive the box in a browser against a local or prod API.
