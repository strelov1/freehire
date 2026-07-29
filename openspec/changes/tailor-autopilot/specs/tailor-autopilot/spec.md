## ADDED Requirements

### Requirement: An autopilot run is started by the server, not composed by the client

The system SHALL expose a dedicated endpoint that starts an unattended tailoring run on a tailoring
session, streaming the turn exactly as an ordinary message does. The server MUST own both the brief
that opens the run and the turn's tool-call ceiling; the client sends no prompt text and no limit.
The endpoint MUST refuse a session that is not a tailoring session, that carries no bound CV, or that
belongs to another user, reporting a session the caller does not own as missing rather than forbidden.

#### Scenario: A run starts on a tailoring session

- **WHEN** the owner of a tailoring session with a bound CV starts an autopilot run
- **THEN** a turn runs on that session and streams the same named events an ordinary message streams

#### Scenario: A non-tailoring session is refused

- **WHEN** an autopilot run is requested on a session whose preset is not the tailoring preset
- **THEN** the request is rejected and no turn is started

#### Scenario: A foreign session is reported missing

- **WHEN** an autopilot run is requested on a session belonging to another user
- **THEN** the request is reported as not found and no turn is started

### Requirement: A run walks every requirement without interrupting the candidate

The agent SHALL, during an autopilot run, work through every requirement of the vacancy's fit
analysis in one turn: searching the experience bank for each, and rewriting the tailored CV where the
bank holds evidence. It MUST NOT ask the candidate anything until the run is finished — a requirement
it cannot close from the bank is carried to the report instead of becoming a mid-run question. Every
bullet written during a run MUST cite the evidence it came from, under the same provenance rule that
governs a hand-driven edit; a run MUST NOT be able to write an unevidenced claim.

#### Scenario: Evidence in the bank becomes a tailored bullet

- **WHEN** a run reaches a requirement the experience bank has evidence for
- **THEN** the agent rewrites the CV to surface that evidence in the vacancy's language, citing the evidence it used

#### Scenario: A requirement with no evidence is deferred, not asked

- **WHEN** a run reaches a requirement the experience bank has nothing for
- **THEN** the agent records it as unclosed and continues to the next requirement without addressing the candidate

#### Scenario: An unevidenced bullet is refused during a run

- **WHEN** an edit made during a run attempts to write a bullet citing no evidence
- **THEN** the edit is rejected and the CV is unchanged

### Requirement: A run records a per-requirement report on the tailored CV

The system SHALL persist, on the tailored CV, a report naming every requirement the run considered
and the outcome of each, so the workspace can show what a run achieved after a page reload. An
outcome MUST be one of a fixed vocabulary — closed from the experience bank, closed from what the
candidate said, still open, or not reached — and a value outside that vocabulary MUST be refused with
a message naming the valid ones rather than persisted. The report tool MUST replace the whole report
on each call and MUST return a receipt rather than echoing the report back into the conversation.

#### Scenario: The report survives a reload

- **WHEN** a run finishes and the workspace is reloaded
- **THEN** the report of that run is read back with the same requirements and outcomes

#### Scenario: An out-of-vocabulary outcome is refused

- **WHEN** the agent reports an outcome outside the fixed vocabulary
- **THEN** the call is refused with a message naming the valid outcomes, and the stored report is unchanged

#### Scenario: A later answer updates the report in place

- **WHEN** the candidate confirms experience for an open requirement after the run and the agent writes it into the CV
- **THEN** the report is replaced with one marking that requirement closed from what the candidate said

### Requirement: A run accounts for itself even when it never reports

The system SHALL write the vacancy's requirements onto the tailored CV as not-reached when a
run starts, so a run that ends before reporting still leaves a report behind. This MUST be the
server's doing rather than the agent's: a run that exhausts its tool-call ceiling is asked for
its final answer with no tools offered, so the run that most needs to be accounted for is
exactly the one that cannot report. Whatever the agent reports afterwards replaces the whole
list. A vacancy with no cached analysis to read MUST leave the previous report untouched
rather than fail the run.

#### Scenario: A run that reports nothing still leaves a report

- **WHEN** a run edits the CV and ends without calling the report tool
- **THEN** the CV carries a report naming the vacancy's requirements, each recorded as not reached

#### Scenario: The agent's report replaces the plan

- **WHEN** a run completes and reports its outcomes
- **THEN** the stored report is the agent's, not the not-reached list the run started from

### Requirement: A run is revertable in one move

The system SHALL snapshot the tailored CV's document before a run makes its first edit, and SHALL
offer an owner-scoped revert that restores that snapshot. A revert MUST clear both the snapshot and
the report, because a report describing edits that no longer exist misdescribes the CV. A CV with no
snapshot MUST report that there is nothing to revert rather than altering the document.

#### Scenario: Reverting restores the pre-run document

- **WHEN** the owner reverts after a run
- **THEN** the tailored CV's document is the one it had before the run started

#### Scenario: Reverting clears the run's traces

- **WHEN** a revert completes
- **THEN** both the snapshot and the report are cleared, and the workspace offers to start a run again

#### Scenario: Reverting without a run is refused

- **WHEN** a revert is requested for a CV that has no snapshot
- **THEN** the request is refused and the document is unchanged

### Requirement: A run ends with a summary and one question

The agent SHALL close an autopilot run by writing a short summary of what it changed and asking about
the FIRST unclosed requirement only. It MUST NOT present the unclosed requirements as a list of
questions; the remaining ones are visible in the report, and answering them is an ordinary
conversation in which each confirmed answer is banked before it is written into the CV.

#### Scenario: The run's last message is a single question

- **WHEN** a run finishes with several unclosed requirements
- **THEN** the agent's closing message summarises the run and asks about one of them

#### Scenario: A confirmed answer is banked before it is written

- **WHEN** the candidate confirms real experience for an unclosed requirement
- **THEN** their own words are recorded in the experience bank first, and the CV bullet cites that record

### Requirement: The workspace shows the run report beside the fit analysis

The workspace SHALL render the run report in its right-hand panel above the existing fit analysis,
showing each requirement with its outcome, and SHALL offer starting another run and reverting the last
one from that block. Starting a run MUST be offered whether or not a report exists — a CV that has
just been reverted has no report by design, and one whose run stopped early may have none either —
while reverting is offered exactly while a snapshot is held. The report MUST arrive with the CV the workspace already re-reads after a turn,
without an additional poll, and the fit analysis it sits above MUST NOT be recomputed by a run.

#### Scenario: The report appears after a run

- **WHEN** an autopilot run finishes
- **THEN** the workspace's Verdict panel shows each requirement with its outcome above the fit analysis

#### Scenario: Another run can be started after an undo

- **WHEN** the last run has been undone and the report cleared
- **THEN** the panel still offers to start a run

#### Scenario: The fit analysis is left alone

- **WHEN** a run finishes
- **THEN** the cached fit analysis shown beneath the report is the same one shown before the run
