## ADDED Requirements

### Requirement: Banking an atom folds its skills into an existing search profile

Whenever an atom is created or updated in the bank, through any entry point (CV-upload
import, the `experience_add`/`experience_update` chat tools, or any future direct-add
surface), the system SHALL fold the atom's canonical skills into the owner's search
profile, if and only if the owner already has a saved profile. It SHALL NOT create a
profile that does not exist. The fold SHALL apply the same merge rule a manual claim
already applies: case-insensitive de-duplication, skipping any skill present in the
profile's `excluded_skills`, and never exceeding the profile skill cap. A failure to fold
SHALL NOT fail the atom write — the atom is durably banked regardless, and the fold is
best-effort.

#### Scenario: Atom banked via CV upload for a user with a saved profile

- **WHEN** a CV upload imports a new atom whose canonical skills include `kubernetes`, and
  the owner's saved profile does not yet list `kubernetes` and does not exclude it
- **THEN** the atom is persisted, and the owner's profile skills come to include
  `kubernetes`

#### Scenario: Atom banked via chat for a user with no saved profile

- **WHEN** the `experience_add` tool banks a new atom for a user who has never saved a
  search profile
- **THEN** the atom is persisted, and no profile is created as a side effect

#### Scenario: A skill the profile has excluded is not re-added

- **WHEN** an atom is banked whose canonical skills include a skill the owner's profile
  currently lists in `excluded_skills`
- **THEN** the atom is persisted, and that skill is NOT added to the profile's `skills`

#### Scenario: Folding a skill does not remove skills the profile already excludes elsewhere

- **WHEN** an atom is banked naming skills already present in the profile's `skills`
- **THEN** the fold is a no-op for those skills — no duplicate entries, and the profile's
  `excluded_skills` and `specializations` are unchanged

#### Scenario: A skill removed from every atom is not retroactively removed from the profile

- **WHEN** an atom is updated in a way that drops a skill it previously listed, and no
  other atom names that skill
- **THEN** the profile's `skills` still lists that skill — folding only ever adds, it never
  removes a skill on the owner's behalf
