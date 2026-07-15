## Why

Every inbox email is classified by an LLM call. An audit of 240 real labeled
emails showed a tuned keyword classifier agrees with the LLM ~80% of the time,
and the confident cases (explicit "unfortunately/regret" rejections, clear
acknowledgement templates, explicit interview invites, offers) are highly
deterministic. For **auto-linked** emails the LLM is called *only* to get the
status — so a confident keyword status there skips the LLM entirely, cutting a
large share of classification calls at no accuracy cost on those cases.

## What Changes

- Add a precision-first deterministic `KeywordStatus(subject, body)` in
  `mailclassify` that returns a signal only on strong, unambiguous phrases and
  otherwise defers (empty, false) — it must NOT fire on the ambiguous
  acknowledgement/rejection boundary.
- `Classifier.Classify` takes a fast-path: when no disambiguation is needed (no
  candidates offered — the auto-linked case) and `KeywordStatus` is confident, it
  returns that status without calling the LLM. The ambiguous tail and any email
  needing candidate disambiguation still go to the LLM unchanged.

## Capabilities

### Modified Capabilities
- `email-body-classification`: add the deterministic keyword fast-path before the
  LLM status classification.

## Impact

- `internal/mailclassify/keyword.go` (new) + `classifier.go` (fast-path).
- No API/DB/prompt/vocabulary change; the LLM path and controlled vocabulary are
  untouched. Fewer LLM calls, same outputs on the cases it fires.
