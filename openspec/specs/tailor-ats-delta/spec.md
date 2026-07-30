# tailor-ats-delta Specification

## Purpose
Tell a candidate what tailoring did to their CV's ATS readiness, measured on the artifact an ATS
actually parses — the rendered PDF's text layer — rather than on the document we meant to render.
The tailored copy is compared against the base CV it was copied from with the template, the page
margins and the keyword baseline held identical, so the reported difference is the content's
contribution and nothing else. The signal is informational: it gates nothing, and it is served
only to the candidate, never to the tailoring agent whose output it measures.

## Requirements
### Requirement: A tailored CV is scored from its rendered artifact

The system SHALL score a tailored CV by rendering it to PDF with its own template and page
margins, extracting the PDF's text layer, and running the deterministic ATS score over that text.
It MUST NOT score the stored CV document directly: the document is what we meant, the text layer is
what a parser reads, and only the latter can expose a layout or reading-order regression.

The CV's own skill set SHALL be parsed from the same extracted text (the existing résumé-acronym
skill tagging), so the score describes the artifact and nothing outside it.

#### Scenario: The score reads the rendered text layer
- **WHEN** a tailored CV is scored
- **THEN** the score is computed from text extracted from the rendered PDF, and a document whose
  rendered text layer is empty (a template producing no extractable text) fails machine-readability
  rather than scoring as if the JSON had been read

#### Scenario: The stored document is not the scoring input
- **WHEN** the stored document contains a field the active template does not render
- **THEN** that field contributes nothing to the score

### Requirement: The comparison holds everything but the tailoring constant

The delta SHALL compare the tailored CV against the base CV it was copied from, with every scoring
input other than the document content held identical: the base CV SHALL be rendered with the
**tailored copy's** template and page margins, and both sides SHALL be scored against the **same**
keyword baseline — the canonical skills of the vacancy the tailored CV is bound to.

The base CV's own template and margins SHALL NOT be used, and the base CV SHALL NOT be modified in
any way by scoring it.

#### Scenario: A template difference does not leak into the delta
- **WHEN** the base CV's stored template differs from the tailored copy's
- **THEN** both sides are rendered with the tailored copy's template, so the delta reflects content
  only

#### Scenario: Both sides share one keyword baseline
- **WHEN** the delta is computed for a CV bound to a vacancy
- **THEN** the vacancy's canonical skills are the keyword baseline for both the base and the
  tailored score

#### Scenario: Scoring the base CV leaves it untouched
- **WHEN** the delta is computed
- **THEN** the base CV's stored document, template and margins are unchanged afterwards

### Requirement: The delta reports the overall and per-category change

The response SHALL carry, for the overall score and for each of the five ATS categories, the base
value, the tailored value, and their signed difference. The category set and its labels SHALL be
the ones the ATS score already defines — this change introduces no new category and no new
weighting.

#### Scenario: Every category carries before, after and difference
- **WHEN** a delta is served
- **THEN** the overall and each of the five categories report base, tailored, and a signed delta,
  and each category's delta equals its tailored value minus its base value

#### Scenario: An unchanged document reports a zero delta
- **WHEN** the tailored CV's content is identical to the base CV's
- **THEN** every reported delta is zero

### Requirement: A drop in ATS readability is reported as a warning

When the tailored CV's overall score is **lower** than the base CV's, the response SHALL mark the
delta as a regression and name the category that fell furthest, so the candidate learns not only
that the CV got worse but where. A score that rose or held SHALL NOT be marked as a regression.

The warning is informational. The system MUST NOT block, gate, or require confirmation for any
action because of it — rendering, downloading, or exporting the tailored CV SHALL behave exactly as
it does without the delta.

#### Scenario: A regression names its worst category
- **WHEN** the tailored overall score is below the base overall score
- **THEN** the response is marked as a regression and names the category with the most negative
  delta

#### Scenario: An improvement is not a regression
- **WHEN** the tailored overall score is equal to or above the base overall score
- **THEN** the response is not marked as a regression and names no category

#### Scenario: A regression gates nothing
- **WHEN** the delta is a regression
- **THEN** the tailored CV's PDF still renders and downloads without confirmation

### Requirement: The delta is computed on read and never stored

The delta SHALL be computed per request from the current documents, the current template and the
current scoring rules, and no part of it SHALL be persisted. A subsequent scoring-rule change or
document edit SHALL therefore be reflected on the next read with no migration or invalidation step.

#### Scenario: A repeated read of an unchanged CV is identical
- **WHEN** the delta is read twice with no edit in between
- **THEN** both responses are identical

#### Scenario: An edit is reflected on the next read
- **WHEN** the tailored CV is edited and the delta is read again
- **THEN** the new response reflects the edit, and no stored delta had to be invalidated

### Requirement: The delta is owner-scoped and only defined for a tailored CV

The read SHALL be owner-scoped: a CV belonging to another account SHALL be indistinguishable from
one that does not exist. A CV with no defined comparison SHALL be refused as a conflict, not
answered with a fabricated baseline. Two distinct cases have no comparison, and the refusal SHALL
say which one it is rather than describing both as "not a tailored CV":

- the CV **is** the base CV — there is nothing to compare it against;
- the CV is a tailored copy whose **vacancy no longer exists** (pruned), so the keyword baseline
  both sides would be scored against is gone.

The baseline SHALL be the CV marked as the user's base. A vacancy-less tailored copy MUST NOT be
used as the baseline, however recently it was edited.

#### Scenario: Another account's CV is not found
- **WHEN** a caller reads the delta for a CV owned by a different account
- **THEN** the response is the same not-found as for a CV id that does not exist

#### Scenario: A base CV has no delta
- **WHEN** a caller reads the delta for the base CV
- **THEN** the response is a conflict saying it is the base, and no score is computed

#### Scenario: A tailored copy whose vacancy was pruned has no delta
- **WHEN** a caller reads the delta for a tailored copy whose vacancy row has been deleted
- **THEN** the response is a conflict saying the vacancy no longer exists, and no score is computed

#### Scenario: An orphan is never the baseline
- **WHEN** the user has a base CV and an orphaned tailored copy edited more recently, and a delta
  is read for a third, live tailored CV
- **THEN** the comparison is against the base CV, and the response names it as the baseline

### Requirement: An unavailable renderer degrades the delta, not the workspace

When the delta cannot be computed for an environmental reason — the Typst renderer or the PDF text
extractor is unavailable, or a render fails — the response SHALL report the delta as unavailable
with a reason, with a success status. It MUST NOT return a server error and MUST NOT prevent the
tailoring workspace from loading or the CV from being edited.

#### Scenario: A missing renderer yields an unavailable delta
- **WHEN** the CV renderer is not configured in the running environment
- **THEN** the response reports the delta as unavailable with a reason and a success status

#### Scenario: A failed render does not fail the workspace
- **WHEN** rendering either side fails
- **THEN** the delta is reported unavailable and the workspace continues to load and edit the CV

### Requirement: The workspace surfaces the delta without being asked

The tailoring workspace SHALL request the delta when it opens and again after an autopilot run
completes — the two moments the document is most likely to have changed — and display the overall
change, the per-category breakdown, and the regression warning when there is one. The candidate
SHALL NOT have to trigger a check to be told their CV got worse.

An unavailable delta SHALL render as an absence, not as an error state.

#### Scenario: Opening the workspace shows the delta
- **WHEN** the tailoring workspace opens for a tailored CV
- **THEN** the delta is requested and its overall change and per-category breakdown are displayed

#### Scenario: A completed run refreshes the delta
- **WHEN** an autopilot run finishes
- **THEN** the delta is requested again and the displayed values reflect the run's edits

#### Scenario: An unavailable delta shows nothing rather than an error
- **WHEN** the delta is reported unavailable
- **THEN** the workspace displays no delta and no error state
