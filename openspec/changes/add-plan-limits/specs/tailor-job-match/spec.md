## MODIFIED Requirements

### Requirement: The job-match score measures the tailored document against its vacancy alone

The system SHALL score a tailored CV against the vacancy it is bound to, using only that vacancy
and the tailored document as inputs. It MUST NOT read the base CV, the experience bank, or the
candidate's structured résumé: those describe the candidate, and this score describes the
document.

The scoring input SHALL be the text layer extracted from the tailored CV rendered to PDF with its
own template and page margins — the same artifact the ATS-readiness score reads, for the same
reason: a field the active template never renders contributes nothing to what a parser sees.

The score SHALL be deterministic and SHALL NOT call a language model. It therefore consumes no
plan allowance and MAY be recomputed as often as the document changes.

#### Scenario: The score reads the rendered text layer

- **WHEN** the job-match score is computed for a tailored CV
- **THEN** it is computed from text extracted from the rendered PDF, and a document field the active template does not render contributes nothing

#### Scenario: The base CV does not enter the score

- **WHEN** the job-match score is computed
- **THEN** the base CV is neither read nor rendered, and the score is unchanged by any edit to it

#### Scenario: Scoring makes no model call

- **WHEN** the job-match score is computed with no LLM configured in the environment
- **THEN** the score is produced in full and no allowance is consumed

#### Scenario: Scoring is unaffected by an exhausted allowance

- **WHEN** the job-match score is recomputed for a candidate who has spent every daily allowance
- **THEN** the score is produced in full and is not refused
