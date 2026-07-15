## Context

`Classifier.Classify` always calls the LLM. The runner offers candidates only for
the ambiguous/unmatched tail; an auto-linked email is classified with no
candidates, so the LLM is spent purely on status. `Input.Body` is already the
readable body (subject+body drive the decision).

## Goals / Non-Goals

**Goals:** skip the LLM for auto-linked emails whose status is unambiguous by
keyword; keep precision high (never mislabel the ack/rejection boundary).

**Non-Goals:** replacing the LLM (the ambiguous tail and disambiguation stay on
the LLM); multilingual coverage (non-English defers to the LLM); changing the
vocabulary, prompt, or stage rules.

## Decisions

- **Precision over recall.** `KeywordStatus` fires only on strong signals and
  returns (\"\", false) otherwise. Ordered so a rejection phrase wins over an
  acknowledgement opener (soft rejections that start "thank you for your
  interest…" are caught by the negative phrase, not the ack template). Ambiguous
  openers alone ("thank you for your interest") do NOT trigger acknowledgement.
- **Fast-path only when no candidates.** `Classify` shortcuts only when
  `len(Candidates)==0` (auto-linked / status-only). With candidates present the
  LLM is still needed for `matched_job_id`, so no shortcut.
- **Confidence.** Keyword hits return a high confidence (0.95) so downstream
  stage advancement (threshold 0.8) still fires; `MatchedJobID` stays 0.

## Risks / Trade-offs

- A wrong keyword rule would persist a wrong status without the LLM catching it —
  mitigated by precision-first phrasing and unit tests over representative cases.
- Keyword recall is partial by design; the LLM still handles everything the
  fast-path defers, so overall accuracy is unchanged.
