## MODIFIED Requirements

### Requirement: Profile management UI

The web app SHALL present signed-in users a view at `/my/profile` that shows their
one profile and lets them edit or clear it, and SHALL prompt anonymous users to sign
in instead. There is no profile name and no list of profiles. Before a profile
exists, the view SHALL show a single inline set-up form covering Role
(specializations, capped at 5), CV upload, headshot, and Skills together — the
server requires at least one specialization and at least one skill to create a
profile, so both SHALL be collected before the first save, and the set-up form's
Save control SHALL be enabled exactly when at least one specialization and at
least one skill are present. Once a profile exists, the view SHALL present a
top-level tab strip including a "Settings" tab (CV upload, headshot, Role, and
location & work preferences, saved together via one Save control scoped to that
tab's fields) and a separate "Skills" tab holding the Skills and Skills to avoid
controls. Skill and skill-to-avoid edits on the Skills tab SHALL take effect
immediately (no Save control on that tab) per the "Autosave skill edits on an
existing profile" requirement below. Skills and skills-to-avoid selection SHALL
use a searchable multi-select sourcing its candidates from the live skills facet
endpoint, with a skill never selectable in both sets at once.

#### Scenario: Create the profile from the set-up form

- **WHEN** a signed-in user with no profile yet picks one or more specializations and skills in the set-up form and saves
- **THEN** the app calls `PUT /api/v1/me/profile` and the view switches to the tabbed, existing-profile layout

#### Scenario: Edit Role on an existing profile

- **WHEN** a signed-in user who already has a profile opens the Settings tab
- **THEN** its controls are pre-seeded with their current Role, CV, headshot, and location preferences, and saving them does not require re-entering skills

#### Scenario: Skills use a live facet-backed search control

- **WHEN** a signed-in user edits skills or skills-to-avoid on the Skills tab, or Role in Settings' set-up-time skills field
- **THEN** the control lists canonical skills sourced from the live skills facet endpoint, filtered to the typed query (not a static/bundled list)

#### Scenario: Set-up Save control reflects completeness

- **WHEN** a signed-in user with no profile yet has entered at least one specialization and at least one skill in the set-up form
- **THEN** the Save control is enabled; when either is missing it is disabled

#### Scenario: Anonymous prompt

- **WHEN** an anonymous (signed-out) user opens `/my/profile`
- **THEN** the view shows a "sign in" affordance instead of the profile

### Requirement: Populate profile skills from a resume

The profile view SHALL let a user upload a resume (PDF) from the Settings tab and
extract skills from it via the `resume-skill-extraction` capability, merged as a
union with skills already on the profile (deduplicated) — it SHALL NOT remove or
overwrite skills already present. Before a profile exists, extracted skills merge
into the set-up form's in-memory skills field, editable before the first save.
Once a profile exists, extracted skills — and any specialization the same
extraction resolved — SHALL be written to the profile together, in one
additional `PUT /api/v1/me/profile` beyond whatever the Settings tab itself
saves. Skills and specializations SHALL commit in the SAME write, not two
separate ones: the Settings tab is remounted from the server's own row on any
write to it, so a specialization merged only into this component's local state
by a skills-only write would be discarded, unsaved, before the user could ever
save it through the visible Role field. Extracted skills are visible on the
Skills tab; the user can remove any unwanted extracted skill there afterwards,
same as any other skill.

#### Scenario: Merge extracted skills into an empty set-up form

- **WHEN** a user with no profile and an empty skills field uploads a resume and extraction returns `[go, postgresql]`
- **THEN** the set-up form's skills field contains exactly `[go, postgresql]`, unsaved until the user saves

#### Scenario: Merge extracted skills without wiping existing entries

- **WHEN** a user whose set-up form already has `[docker]` uploads a resume returning `[go, docker, postgresql]`
- **THEN** the form's skills field contains the union `[docker, go, postgresql]` with no duplicates

#### Scenario: Extraction in progress shows a loading state

- **WHEN** the resume is being uploaded and analyzed
- **THEN** the upload control shows a loading/disabled state until extraction completes or fails

#### Scenario: Extracted skills autosave to an existing profile

- **WHEN** a user who already has a profile with skills `[docker]` uploads a resume from the Settings tab returning `[go, docker, postgresql]`
- **THEN** the profile's skills become `[docker, go, postgresql]` immediately, without the user pressing Settings' Save control, and are visible on the Skills tab

#### Scenario: An extraction-resolved specialization commits in the same write as its skills

- **WHEN** a user who already has a profile uploads a resume that resolves both new skills and a specialization not yet on the profile
- **THEN** both land in the profile from that single upload, without the user pressing Settings' Save control

## ADDED Requirements

### Requirement: Autosave skill edits on an existing profile

For a signed-in user who already has a profile, toggling a skill or an avoided
skill on the Skills tab SHALL write to the profile immediately via
`PUT /api/v1/me/profile`, without a Save control on that tab. Claiming a skill
SHALL also stop avoiding it if it was avoided, and avoiding a skill SHALL also
un-claim it if it was claimed — a skill is never in both sets. While a write from
one toggle is in flight, the Skills tab's controls SHALL be disabled so a second
toggle cannot be started against a stale pre-write state. A write that fails
SHALL leave the profile's stored skills unchanged and SHALL show an inline error
naming the skill that failed. The server requires at least one skill on every
save, not only at profile creation — removing the profile's one remaining skill,
or avoiding it (which also un-claims it), would always be rejected. The Skills
tab SHALL refuse that specific toggle client-side, without attempting the write,
and SHALL show a message stating a skill is required rather than the generic
retry error.

#### Scenario: Claiming a skill autosaves

- **WHEN** a signed-in user with an existing profile toggles on a skill on the Skills tab
- **THEN** the app calls `PUT /api/v1/me/profile` with that skill added, with no Save control involved

#### Scenario: Avoiding a skill drops it from the held set

- **WHEN** a signed-in user's profile holds skill `go` and they mark `go` as avoided on the Skills tab
- **THEN** the saved profile's skills no longer include `go` and its excluded_skills does

#### Scenario: A second toggle is blocked while the first is saving

- **WHEN** a signed-in user toggles a skill on the Skills tab and the write to the server has not yet completed
- **THEN** the tab's skill controls are disabled until that write settles

#### Scenario: A failed write is surfaced without silently reverting

- **WHEN** a signed-in user toggles a skill on the Skills tab and the save request fails
- **THEN** the profile's stored skills remain whatever they were before the toggle, and the tab shows an inline error naming the skill

#### Scenario: Removing the one remaining skill is refused client-side

- **WHEN** a signed-in user's profile holds exactly one skill and they try to remove it, or to mark it as avoided, on the Skills tab
- **THEN** no write is sent to the server, the skill remains in the profile's `skills`, and the tab shows a message that a skill is required
