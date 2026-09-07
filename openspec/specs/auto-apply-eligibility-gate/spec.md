# auto-apply-eligibility-gate Specification

## Purpose

Stops the auto-apply pipeline from queuing or completing a submission for a
(candidate, job) pair the candidate's own known work-authorization or location evidence
positively conflicts with, instead of relying on an incidental drafting or form-matching
failure to catch it.

## Requirements

### Requirement: Enqueueing refuses a positively mismatched pair

The system SHALL evaluate the candidate's known work-authorization and
location-and-work-mode evidence against the job's requirements before adding an
auto-apply attempt to the queue, and SHALL refuse to enqueue when that evaluation
reports an unmet work-authorization or location-and-work-mode blocker for the pair. The
refusal response MUST name the blocker's reason rather than reading as a generic
failure.

This evaluation SHALL follow the same "never emit a false blocker" discipline used
elsewhere for this comparison: when the candidate's or the job's evidence for a category
is absent, that category MUST be treated as unevaluable and MUST NOT by itself refuse
enqueueing.

#### Scenario: A job requiring work authorization the candidate does not have is refused

- **WHEN** a candidate whose known work-authorization evidence conflicts with a job's
  stated visa-sponsorship and country requirements requests auto-apply on that job
- **THEN** the attempt is not added to the queue
- **AND** the response names the work-authorization mismatch as the reason

#### Scenario: A job requiring a residency or work mode the candidate does not have is refused

- **WHEN** a candidate whose known location evidence conflicts with a job's required
  country or work mode requests auto-apply on that job
- **THEN** the attempt is not added to the queue
- **AND** the response names the location mismatch as the reason

#### Scenario: Missing evidence on either side does not refuse enqueueing

- **WHEN** the candidate has not stated (and none can be derived for) the evidence a
  category needs, or the job carries no requirement for that category
- **THEN** that category is treated as unevaluable
- **AND** enqueueing proceeds on the strength of the pipeline's other existing checks

#### Scenario: A pair with no conflicting evidence is unaffected

- **WHEN** the evaluation finds no unmet work-authorization or location-and-work-mode
  blocker for the pair
- **THEN** enqueueing proceeds exactly as it did before this gate existed

### Requirement: A required, unmapped geography or residency question never reaches automatic drafting

The system SHALL recognize a required application-form question whose label identifies a
geography, residency, or work-location requirement (distinct from the existing
visa/sponsorship terms) as a form the pipeline cannot safely resolve on its own, before
that question is offered to automatic drafting, and SHALL record the attempt as
unresolved rather than submitting a drafted guess.

#### Scenario: A required residency question with no known answer is parked, not drafted

- **WHEN** a queued attempt's application form carries a required question whose label
  identifies a geography or residency requirement, and the candidate's known answers do
  not map to it
- **THEN** the attempt is recorded as unresolved naming that question
- **AND** no drafted answer is generated or submitted for it

#### Scenario: A mapped or already-answered geography question is unaffected

- **WHEN** a geography-labeled question's answer is already resolvable from the
  candidate's known answers
- **THEN** the question is filled from that known answer exactly as before this rule
  existed, and drafting is not invoked for it
