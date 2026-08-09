## ADDED Requirements

### Requirement: A cited claim can be checked for fidelity to its own evidence

The tailoring agent SHALL have access to a tool that, given the `evidence_id` an edit cited,
returns that atom's own `claim`, `context`, and `metrics` exactly as banked. The tool MUST call no
model and MUST NOT alter the CV, the CV's report, or the experience bank — it is a read, not a
gate. `tailorPrompt` SHALL instruct the agent to call this tool after a batch of edits that cited
evidence, compare the wording it just wrote against what the tool returns, and revise via
`cv_edit` when the written claim states more scope, seniority, or a bigger metric than the atom
supports.

This is independent of the citation requirement: an edit citing a real, publishable atom is
already accepted by the evidence gate regardless of whether this check ever runs. The check is
advisory and agent-directed — bounded the same way the existing `job_match` self-check is bounded
(a soft cap of 2-3 rounds, the agent's own judgment to stop), not a second server-side refusal.
Fidelity is a semantic judgment a deterministic check cannot safely arbitrate; a hard block here
would risk refusing an honest, well-worded bullet.

#### Scenario: The tool returns the cited atom's own text

- **WHEN** the agent calls the fidelity-check tool with an `evidence_id` that names a banked atom
- **THEN** the result carries that atom's `claim`, `context`, and `metrics`, with no model call
  made and no CV, report, or bank state changed

#### Scenario: An overstated bullet is revised after the check

- **WHEN** the agent has written a bullet that claims more scope or seniority than the atom it
  cited supports, and then calls the fidelity-check tool for that citation
- **THEN** the agent's next `cv_edit` call may revise that bullet to stay inside what the atom
  says, and the revision is applied under the same evidence-citation rule as any other edit

#### Scenario: A faithful bullet needs no revision

- **WHEN** the agent calls the fidelity-check tool for a bullet whose wording already stays
  inside what the cited atom says
- **THEN** the agent may proceed without any further `cv_edit` call for that citation, and nothing
  about the CV is changed by the check itself

#### Scenario: The check never blocks a citation the evidence gate already accepted

- **WHEN** an edit cites a real, publishable atom and the fidelity-check tool is never called for it
- **THEN** the edit is still applied exactly as the evidence-citation requirement already governs,
  unaffected by this requirement

#### Scenario: Both tailoring rhythms carry the same rule

- **WHEN** a claim-shaped edit is made during an ordinary conversational tailoring turn or during
  an autopilot run
- **THEN** the same fidelity-check tool and `tailorPrompt` instruction apply in both, since both
  run the same tailoring prompt
