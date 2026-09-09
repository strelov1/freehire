# Answer topics

## Scope
`internal/dict/answertopic` — keys an employer's screening question by what it asks, so one
banked answer serves every phrasing of it.

## Always true
- **The dictionary is an accelerator, never the only route.** A question it does not know
  still keys, from the fold alone. This ordering is deliberate: `internal/api/atsapply`'s
  own `labelAnswerKeyFor` was a sole-route list, and its missing salary rule was invisible
  precisely because nothing reports the questions a list fails to match.
- **A question that folds to nothing is refused, never stored under an empty key.** Real
  forms produce empty labels — Greenhouse's hidden `required` proxy inputs carry no label
  at all — and a key derived from nothing can never be recalled.
- **Two different questions must never collapse.** `desired salary` and `current salary`
  are the worked example: merging them answers one with the other, in the candidate's name,
  to an employer. A test asserts they stay apart; keep it when adding dictionary entries.
- **Pure.** No model, no I/O, no user. A model proposing a topic is out of scope by design.
