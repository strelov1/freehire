## ADDED Requirements

### Requirement: Hidden signals from posting wording

The fit analysis SHALL include a `hidden_signals` field: an array of up to 5 objects, each
carrying a verbatim `quote` excerpted from the job description and a short `insight`
interpreting what that wording implies about pace, ownership expectations, team stage, or
culture. The signals MUST be extracted as part of the existing Stage 1 (Extract & Match) LLM
call — no additional model call or stage. Both `quote` and `insight` MUST be non-empty,
length-bounded, and drawn only from the job description text; the field MUST NOT require or
depend on the candidate's résumé/CV content, even though the analysis as a whole (and therefore
this field) is only produced for a candidate with banked/structured experience.

#### Scenario: Posting wording implies pace and ownership expectations

- **WHEN** a job description repeatedly uses phrasing like "thrive in ambiguity" and "high
  ownership, fast-paced"
- **THEN** the served analysis's `hidden_signals` includes an entry quoting that wording with an
  insight naming the implied self-driven/fast-pace expectation

#### Scenario: Generic posting yields no signals

- **WHEN** a job description is short or carries no distinctive wording to interpret
- **THEN** `hidden_signals` is an empty array, and the analysis is served normally with no error
  and no fabricated signal

#### Scenario: Malformed model output is dropped, not coerced

- **WHEN** the model returns a signal entry with a blank `quote` or a blank `insight`
- **THEN** that entry is dropped from the served `hidden_signals` rather than served with a
  placeholder value

#### Scenario: Signals are cached with the rest of the analysis

- **WHEN** an analysis with `hidden_signals` is served from cache on a subsequent request for the
  same (user, job) with unchanged CV upload time, job `content_hash`, and model
- **THEN** the same `hidden_signals` are served without a new LLM call, under the analysis's
  existing cache stamp
