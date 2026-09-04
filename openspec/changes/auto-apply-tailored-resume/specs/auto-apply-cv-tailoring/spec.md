## Purpose

Lets an external, API-key-authorized caller tailor a CV for one specific auto-apply queue
entry and lets the candidate review and decide on the result before it can ever reach a real
submission — the two steps an automated apply pipeline needs from freehire, without touching
the interactive tailoring surface's own safety posture.

## ADDED Requirements

### Requirement: Enqueuing an attempt reuses the existing deterministic preflight check

The system SHALL run the same deterministic (no-LLM) skill/hard-constraint check
`tailor-preflight-check` already performs before ANY `auto_apply_queue` entry is created for a
vacancy. A hard mismatch SHALL NOT enqueue an entry. The candidate SHALL be offered the
existing interactive tailoring chat for that vacancy so they can add missing experience before
an attempt is queued — this reuses that chat's existing evidence-bank write path
(`stated_in_chat` provenance) rather than introducing a new one.

#### Scenario: A hard mismatch does not enqueue an attempt

- **WHEN** a candidate asks to auto-apply to a vacancy their profile hard-mismatches on the
  deterministic check
- **THEN** no `auto_apply_queue` entry is created, and the candidate is offered the interactive
  tailoring chat for that vacancy instead

#### Scenario: Adding missing experience unblocks enqueueing

- **WHEN** a candidate who was offered the interactive chat adds the missing experience there
  and then asks to auto-apply again
- **THEN** the (now passing, or accepted-anyway) check proceeds to enqueue the entry

### Requirement: Tailoring is scoped to one owned queue entry, not any session

The system SHALL expose an endpoint that starts an unattended tailoring run for exactly one
`auto_apply_queue` entry the caller owns, authenticated by API key. It MUST bootstrap (or
reuse) the tailored CV for that entry's vacancy, run the same unattended autopilot pass the
interactive workspace's autopilot uses, and run it to completion before responding — no
partial/streaming response. It MUST refuse an entry that does not belong to the caller,
reporting it as not found rather than forbidden, and MUST refuse an entry that has already
been reviewed (approved or declined) rather than re-tailoring it.

This endpoint is independent of `POST /assistant/sessions/:id/autopilot`: it does not accept
a caller-supplied session id, it is reachable by API key (that endpoint deliberately is not,
per its own cookie-only rationale), and starting it commits nothing about how its result will
be reviewed.

#### Scenario: Starting tailoring for an owned entry

- **WHEN** an API-key caller starts tailoring for their own auto-apply queue entry
- **THEN** the tailored CV is bootstrapped, the autopilot run completes, and the response
  reports the entry's tailored CV id and its per-requirement report

#### Scenario: A foreign entry is reported missing

- **WHEN** tailoring is requested for a queue entry belonging to another user
- **THEN** the request is reported as not found and no run starts

#### Scenario: An already-reviewed entry is refused

- **WHEN** tailoring is requested again for an entry whose review has already been recorded
  (approved or declined)
- **THEN** the request is refused and no second run starts

### Requirement: The daily tailoring allowance is shared with the interactive workspace

Starting a run through this endpoint SHALL draw on the SAME per-day `tailor` allowance
(`internal/ai/plan`) the interactive tailoring workspace draws on — never a separate or
unmetered counter. A caller with no allowance left SHALL be refused before any CV is
bootstrapped or any model call is made.

#### Scenario: A spent daily allowance refuses the run

- **WHEN** a candidate has no `tailor` allowance left for today and tailoring is requested for
  one of their queue entries
- **THEN** the request is refused, no tailored CV is created, and no autopilot run starts

#### Scenario: Interactive and automated tailoring draw on one counter

- **WHEN** a candidate has tailored a CV interactively today and then this endpoint tailors a
  different vacancy's queue entry the same day
- **THEN** the two runs count against the same daily standing, not two separate ones

### Requirement: The candidate is notified when a tailored CV is ready to review

The system SHALL notify the candidate once a queue entry's tailoring run finishes, with a link
into the existing tailoring workspace on that entry's tailored CV
(`/tailor/[slug]?cv=<id>`), so the candidate can see exactly what changed before deciding.

#### Scenario: A finished run is announced

- **WHEN** a queue entry's tailoring run completes
- **THEN** the candidate receives a notification linking to the tailored CV in the tailoring
  workspace

### Requirement: A review decision is recorded once and gates submission

The system SHALL expose an endpoint that records the candidate's decision — approve or
decline — for one owned, tailored-but-not-yet-reviewed queue entry. Approving marks the entry
eligible for the ATS-submission step (`internal/application/autoapply`), attached to the
tailored CV that was approved. Declining marks the entry parked, with its own distinct reason,
so it is never picked up for submission and never confused with a form-field park. A decision
MAY be recorded only once per entry — a second attempt is refused.

#### Scenario: Approving makes the entry eligible for submission

- **WHEN** a candidate approves their queue entry's tailored CV
- **THEN** the entry becomes claimable by the ATS-submission step, carrying that CV

#### Scenario: Declining parks the entry with its own reason

- **WHEN** a candidate declines their queue entry's tailored CV
- **THEN** the entry is parked with a reason distinct from an unresolved form field, and is
  never claimed for submission

#### Scenario: A decision cannot be recorded twice

- **WHEN** a review decision is requested for an entry that already carries one
- **THEN** the request is refused and the existing decision is unchanged

#### Scenario: An unreviewed entry is never submitted

- **WHEN** a queue entry has a tailored CV but no recorded review decision
- **THEN** the ATS-submission step does not claim it
