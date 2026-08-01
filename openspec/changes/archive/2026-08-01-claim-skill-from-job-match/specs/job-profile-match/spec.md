## ADDED Requirements

### Requirement: Claiming a missing skill from the match block

In the real-match state, every chip in the **Missing** and **Close** groups SHALL be an activatable
control that discloses a claim row naming that skill and offering to add it to the caller's profile.
Confirming SHALL write the skill into the caller's profile skill set and MUST NOT write to the stored
CV, the structured résumé, or the experience bank. Chips in the **You have** group SHALL remain inert:
this affordance adds skills and never removes them. The claim affordance SHALL be absent from the
guest, no-profile, no-skills and loading states, whose chips are a fabricated teaser rather than a
match.

#### Scenario: Missing chip discloses the claim row

- **WHEN** a signed-in viewer with a profile presses the `entra-id` chip in the Missing group
- **THEN** a claim row naming `entra-id` SHALL appear with an action to add it to the profile, and the
  pressed chip SHALL report itself as expanded to assistive technology

#### Scenario: Close chip discloses the same claim row

- **WHEN** the viewer presses a Close chip — a skill they hold only through a neighbour, such as
  `azure · you have aws`
- **THEN** the claim row SHALL offer to add `azure` itself, not the neighbour it was matched through

#### Scenario: Only one claim row is open at a time

- **WHEN** a claim row is open for one skill and the viewer presses a different Missing or Close chip
- **THEN** the row SHALL move to the newly pressed skill, and pressing the same chip again SHALL close it

#### Scenario: Held chips offer nothing

- **WHEN** the viewer presses a chip in the You have group
- **THEN** no claim row SHALL open and the profile SHALL NOT change

#### Scenario: A locked viewer cannot claim

- **WHEN** an unauthenticated viewer, or a signed-in viewer with no profile skills, sees the blurred
  teaser
- **THEN** its chips SHALL NOT be activatable and no claim row SHALL be reachable

### Requirement: The claimed skill is reclassified before the write settles

Confirming a claim SHALL move the skill into the **You have** group and recompute the coverage
percentage and the two-colour bar client-side, without waiting for the profile write. The client-side
recomputation SHALL use the same weighting as the server — an exact match weighs 1, an adjacent match
one half — so the optimistic figure agrees with the server's for the skills it can classify. Once the
write succeeds the block SHALL refetch the match and render the server's classification in place of the
optimistic one, because a claim can also turn a third skill from missing to close through the adjacency
dictionary, which the client does not hold.

#### Scenario: Chip moves and the percentage rises immediately

- **WHEN** the viewer confirms a claim for a Missing skill of a job carrying 4 skills, 1 of which was
  already exact
- **THEN** the skill SHALL appear in You have, the reported counts SHALL read 2 of 4, and the coverage
  SHALL read 50% before the profile request settles

#### Scenario: A claimed Close skill stops being half-weighted

- **WHEN** the viewer claims a skill that was classified adjacent
- **THEN** it SHALL leave the Close group, the adjacent count SHALL fall by one, the exact count SHALL
  rise by one, and the coverage SHALL rise by the half weight the adjacency was contributing

#### Scenario: The server's classification replaces the optimistic one

- **WHEN** the profile write succeeds
- **THEN** the block SHALL refetch the match and render the returned classification, so a skill the
  claim newly made adjacent is shown as Close rather than left in Missing

#### Scenario: A failed refetch keeps the optimistic view

- **WHEN** the profile write succeeds but the follow-up match request fails
- **THEN** the block SHALL keep showing the optimistic classification and MUST NOT revert the claim,
  which the server has already accepted

### Requirement: A claim is confirmed and reversible

After a successful claim the block SHALL show a confirmation naming the skill and offering to undo it.
The confirmation SHALL name the most recent claim; a further claim replaces it. Undo SHALL subtract
only that skill from the profile skill set — never restore a whole earlier profile, which would roll
back any claim made after it — and SHALL restore the block to the classification it had before that
claim. A claimed skill SHALL also be dropped from the profile's excluded-skills set, so the profile
cannot simultaneously claim and avoid the same skill; undo SHALL NOT restore that exclusion.

#### Scenario: Confirmation offers undo

- **WHEN** a claim for `entra-id` is written successfully
- **THEN** the block SHALL show that `entra-id` was added to the profile, with an undo action

#### Scenario: A second claim takes over the confirmation

- **WHEN** the viewer claims `entra-id` and then claims `powershell`
- **THEN** the confirmation SHALL name `powershell`, and undoing it SHALL leave `entra-id` in the
  profile

#### Scenario: Undo returns the skill to the group it came from

- **WHEN** the viewer undoes a claim for `entra-id`
- **THEN** `entra-id` SHALL return to the Missing group and the coverage SHALL read what it did
  before the claim

#### Scenario: Claiming an excluded skill resolves the contradiction

- **WHEN** the viewer claims a skill their profile currently lists among its excluded skills
- **THEN** the saved profile SHALL carry that skill among its skills and no longer among its excluded
  skills

### Requirement: Concurrent claims cannot drop each other

Because the profile endpoint replaces the whole profile row, claims SHALL be serialised so that each
write is built from the result of the previous one. Two claims confirmed in quick succession SHALL both
survive.

#### Scenario: Two rapid claims both persist

- **WHEN** the viewer confirms a claim for `bash` and, before that request settles, confirms one for
  `powershell`
- **THEN** the profile SHALL end up holding both skills, and neither write SHALL be sent with a skill
  list that predates the other

### Requirement: A failed claim is rolled back and reported

When the profile write fails, the block SHALL return the skill to the group it came from, restore the
previous counts and coverage, and state that the skill could not be added. It MUST NOT show the
confirmation or the undo action for a claim that did not land.

#### Scenario: Write failure restores the chip

- **WHEN** the profile write for a claimed skill fails
- **THEN** the skill SHALL reappear in the group it was claimed from, the coverage SHALL return to its
  previous value, and an error naming the failure SHALL be shown
