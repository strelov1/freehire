# The screening answer bank

## Scope
`internal/candidate/answerbank` — what the candidate answers to employers' screening
questions, accumulating across applications.

## Always true
- **The bank is a store, not a cache.** It accumulates; only its owner removes anything.
  No reconciler prunes it, no import replaces it — same rule as
  [the experience bank](../experience/AGENTS.md), and a sweeper here would silently discard
  answers the candidate expects to still hold.
- **Provenance is derived from the ENTRY POINT, never from the request body.** A caller
  naming itself is not evidence of who it is. Only `candidate` may be sent to an employer;
  anything unrecognised fails closed and is not sendable.
- **`List` shows everything, `Sendable` filters.** The management surface must show an
  agent's suggestion so the candidate can confirm it; the application path must not send
  one. Two methods, so the difference cannot be forgotten at a call site.
- **Keyed by topic, not by wording** (`internal/dict/answertopic`). Keying on wording is
  what fills a bank with near-duplicates and asks the same question once per employer.
- **It does not replace `screening_answers`.** Those six facts are typed because the product
  compares and validates them; this takes the open-ended remainder. See migration 0155.
