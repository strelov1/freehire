# Answer topics

## Scope
`internal/dict/answertopic` — keys an employer's screening question by what it asks, so one
banked answer serves every phrasing of it.

## Always true
- **The dictionary is an accelerator, never the only route.** A question it does not know
  still keys, from the fold alone. This ordering is deliberate: `internal/api/atsapply`'s
  own `labelAnswerKeyFor` was a sole-route list, and its missing salary rule was invisible
  precisely because nothing reports the questions a list fails to match.
- **A question that folds to nothing is refused, never stored under an empty key.** A label
  of pure punctuation is what real forms produce, and a key derived from nothing can never
  be recalled.
- **A labelless field is not that case.** Greenhouse's DOM-only inputs carry no label —
  `country`, required on nearly every posting, is the commonest of them — and the caller
  keys those by the field's own id (`internal/api/atsapply`'s `questionText`), which is also
  the text the review screen titles the input with. This package sees "country" and keys it.
  It refused for a while, and the effect was that the single most frequent parking question
  in the product was the one question the bank could never learn.
- **The fold is Unicode, not ASCII.** This catalogue aggregates Russian- and
  Hungarian-language sources, so a question genuinely arrives in one. An ASCII range refuses
  every such question as "no readable text" — and keeps whatever Latin fragment it carried,
  so `Зарплата (USD)` folded to `usd` and pulled every other question mentioning that
  currency onto one topic. The alphabet is the curated-list trap in another shape.
- **Two different questions must never collapse.** `desired salary` and `current salary`
  are the worked example: merging them answers one with the other, in the candidate's name,
  to an employer. A test asserts they stay apart; keep it when adding dictionary entries.
- **Pure.** No model, no I/O, no user. A model proposing a topic is out of scope by design.
