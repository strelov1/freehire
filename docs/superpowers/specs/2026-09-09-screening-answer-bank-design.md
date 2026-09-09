# The screening answer bank

**Status:** design, approved 2026-09-09
**Problem owner:** auto-apply — a queued application parks on questions the candidate has
already answered somewhere, or could answer in seconds, and nothing accumulates.

## The problem, measured

Production, 2026-09-08. Queue entry 3 (Garner Health, Greenhouse) resolved seven of its
required questions and parked on the rest. Three of those are real questions:

| Question | Why it parked |
|---|---|
| What is your desired salary? | The candidate HAS this: `screening_answers` holds 5000 USD/year. No rule connects the two. |
| Which state do you currently reside in? | Nothing holds it, and geography is deliberately never drafted. |
| Are you legally authorized to work in the USA? | Correctly refused: `authorized_countries = {br}` answers a different country. |

The first is the shape this design is about. `resolve.go` matches a question to a stored fact
two ways: by field id (`answerKeyFor`, 17 entries) and by label keywords
(`labelAnswerKeyFor`, **two** entries — visa sponsorship and LinkedIn). A Greenhouse custom
question carries an opaque numeric id (`question_19869712004`), so only a label rule can
reach it, and there is no salary rule. The fact is stored, the question is asked, and
nothing joins them.

Adding a salary rule fixes this one question. It does not fix the next one. Employers author
questions faster than anyone writes rules — the list is a ceiling, and this repository has
been bitten by exactly this shape of hand-maintained list before.

### What is already correct and must stay

- **`salary` is on `sensitive.go`'s list, so a model may never draft one.** Correct: a
  model inventing a candidate's compensation expectation is not a smaller error than
  leaving the field blank. The bank does not relax this. Sensitivity forbids *guessing*;
  it says nothing about using an answer the candidate themselves gave.
- **Work-authorization questions are never auto-answered.** `authorized_countries` is a list
  of countries the candidate may work in, not a yes/no about *this posting's* country.
  Unchanged.
- **Non-sensitive free-text questions already draft** (`ResolveWithDrafting`). The bank does
  not replace drafting; it covers what drafting must not touch.

## Scope decision: two layers, not one

`screening_answers` is not empty and not legacy. Measured on production the same day:

```
users:                     1913
screening_answers rows:     322
  ...with a desired salary: 290
candidate_survey rows:      366
```

It also carries structure the product uses: desired salary is `(amount, currency, period)`
so it compares against `candidate_survey`'s current income **without conversion**, and
`authorized_countries` is an array validated against a country vocabulary. Flattening those
into free text would lose both properties to buy uniformity nobody consumes.

So: **typed facts stay typed; the bank holds everything else.** The two are read through one
interface, so the resolver never learns which it got an answer from.

```
screening_answers (typed facts)   ─┐
                                   ├─→ AnswerSource ─→ resolve.go
screening_answer_bank (free Q&A)  ─┘
```

## Where it lives

`internal/candidate/answerbank` — the store and its provenance rule. It is the candidate's
own record of what they answer, which is `candidate` (layer 4) by the block table's own
reading, beside the experience bank it is modelled on.

The topic fold is `internal/dict/answertopic` (layer 2), separate because it is a pure
normalisation over text with no user in it — the same separation `internal/dict/skilltag`
already has from the candidate data that consumes it. `internal/api/atsapply` (layer 8) and
`cmd/auto-apply` both read the bank through `candidateprofile`'s existing assembler, so the
resolver gains no new dependency of its own.

Both packages go in the layering table (`internal/platform/arch/layering/blocks.go`) — a
package in neither fails the guard.

`internal/ingest/screeninganswers` is left where it is. Moving it is a separate change with
its own risk, and nothing in this design needs it moved.

## Data

```sql
CREATE TABLE public.screening_answer_bank (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    topic      text        NOT NULL,
    question   text        NOT NULL,
    answer     text        NOT NULL,
    provenance text        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (user_id, topic)
);
```

- **`topic`** is what an answer is keyed by, so one answer serves every phrasing of the same
  question. See below.
- **`question`** is the employer's own wording, kept verbatim. It is what the candidate
  actually read when they answered, and it is the only way to audit a stored answer later:
  a topic alone cannot show what was being asked.
- **`UNIQUE (user_id, topic)`**, not `(user_id, question)`. Keying on the wording is what
  makes a bank fill with near-duplicates and ask the same thing repeatedly.
- **`provenance`** — see below. A plain text column matching the experience bank's own
  convention, validated in Go rather than by a CHECK, so adding a vocabulary member stays a
  code change.

## Topic: how phrasings collapse

Two passes, in order:

1. **A small dictionary of known topics** (`internal/dict/answertopic`): salary
   expectation, notice period, relocation, start date, and the handful that recur across
   every board. Keyword rules, the shape `labelAnswerKeyFor` already uses.
2. **Otherwise, the folded question text**: lowercased, punctuation and repeated whitespace
   removed, and a leading politeness wrapper dropped (`please tell us`, `could you`,
   `we'd like to know`). `"What is your desired salary?"` and `"Desired salary"` fold to the
   same key without anybody adding a rule.

The dictionary is an accelerator, never the only path. That ordering is the point: a
hand-maintained list that is the *sole* route is a list whose gaps are invisible, which is
exactly how the salary question got missed. Here a missing dictionary entry costs a
coarser key, not a lost answer.

Folding is deterministic and pure — no model. A model may later *propose* a topic for a
question the fold keys awkwardly, but that is not in this design.

## Provenance: who is speaking

Directly modelled on `internal/candidate/experience`, and for the same reason: an answer
sent to an employer is a claim made in the candidate's name.

| Provenance | Set by | May be sent to an employer |
|---|---|---|
| `candidate` | the person, from their own session or API key | **yes** |
| `agent_inferred` | a model's reading, stored as a suggestion | no |

Like the experience bank, the label is **derived from the entry point**, never taken from
the request body — a caller naming itself is not evidence of who it is. Anything
unrecognised falls to `agent_inferred`, which cannot be sent. Failing closed is the point.

Only `candidate` answers enter `AnswerSource`. An `agent_inferred` row is a suggestion the
review screen may show; promoting it requires the candidate saving it, which re-stamps it.

**This design writes only `candidate` rows.** `agent_inferred` exists in the vocabulary from
the start so that the gate is built before there is anything to let through — adding the
value later would mean adding the enforcement later too, to code that had never needed it.

## Flow

1. `auto-apply`'s preview pass resolves a form. Unanswered required questions land in
   `unmapped` — unchanged.
2. The review screen renders each with an input, alongside the answers that DID resolve.
   The candidate is already on this screen to approve the application; this is the moment
   they are looking at the gap.
3. Saving posts `{question, answer}`; the server derives the topic and stores the row with
   `provenance = candidate`.
4. The next resolve — this application's or any other's — finds it by topic.

Answering does not itself re-run the application. The entry keeps its existing path
(re-preview on the next drain), so nothing about the approval flow changes.

## Surfaces

- `GET /api/v1/me/answer-bank` — list, newest first.
- `PUT /api/v1/me/answer-bank` — upsert one `{question, answer}` by derived topic.
- `DELETE /api/v1/me/answer-bank/:id` — the owner removes; nothing else ever does.

Cookie or a full-scope API key, matching the rest of `/me`. The CLI
(`strelov1/freehire-cli`, a separate repository) needs no server work beyond these routes —
it authenticates with an API key like any other client. Its own commands ship there.

The bank accumulates and only its owner removes anything: no reconciler prunes it, and an
import never deletes. Same rule as the experience bank, stated here so a future sweeper is
recognised as the mistake it would be.

## Errors and edges

- **An empty answer is a delete, not a stored blank.** A blank string in a form field is
  indistinguishable from an unanswered question, and storing one would silently mark a
  question answered forever.
- **A question whose fold yields an empty topic is refused** (a label of pure punctuation,
  which real forms do produce). It cannot be keyed, so it cannot be recalled.
- **Answer length is bounded at 2000 characters.** Long enough for any screening answer a
  form actually asks for (the longest real one measured is a two-sentence "why this
  company"), short enough that a pasted essay is refused rather than stored — that is a
  cover letter, and there is a separate path for those. The bound is enforced in the
  service, not only in the form, since an API key reaches this route too.
- **The bank never overrides a typed fact.** If `screening_answers` answers a question, that
  wins: it is validated and structured, the bank's copy is free text. The merge order is
  fixed, not a per-answer decision.

## Testing

- `answertopic`: fold cases from real captured labels (the three in the table above, plus
  the four Greenhouse proxy inputs' empty labels as the refusal case). Table-driven, pure.
- Provenance gate: a row stored through a non-candidate entry point never reaches
  `AnswerSource`. The experience bank's own test for this is the model.
- Integration: a queue entry parks on a question, an answer is banked, the next resolve
  fills it — the whole point of the feature, asserted end to end.
- The salary case specifically, as a regression: the fact is in `screening_answers` and the
  question phrases it three different ways.

## Not in this design

- A model proposing answers or topics (`agent_inferred` writes). The vocabulary and the gate
  are built; the writer is not.
- Backfilling the bank from anything. It starts empty and fills as applications run.
- Re-running an application automatically when its blocking question is answered.
