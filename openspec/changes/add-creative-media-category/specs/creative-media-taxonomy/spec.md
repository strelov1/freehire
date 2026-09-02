## ADDED Requirements

### Requirement: Media production is a category of its own

The system SHALL resolve media-production work — video, animation, art, audio
and photography — to a dedicated `creative` category. The category MUST be a
member of the technical category set, so its postings derive `is_tech = true`
and are enqueued for AI enrichment and semantic embedding like `design` and
`product`. It MUST NOT be a member of the non-technical set, which is what
`cmd/prune` deletes from, so no creative posting becomes removable by
resolving.

#### Scenario: A video title resolves to the creative category

- **WHEN** a job titled "Video Editor", "Senior Video Producer" or
  "Videographer" is classified
- **THEN** its category is `creative`

#### Scenario: An art or animation title resolves to the creative category

- **WHEN** a job titled "3D Artist", "Concept Artist", "Technical Artist",
  "Character Artist", "Environment Artist", "VFX Artist", "Storyboard Artist",
  "Animator" or "3D Animator" is classified
- **THEN** its category is `creative`

#### Scenario: A photography title resolves to the creative category

- **WHEN** a job titled "Photographer" or "Photo Editor" is classified
- **THEN** its category is `creative`

#### Scenario: The creative category is technical

- **WHEN** a job resolves to `creative`
- **THEN** its derived `is_tech` is `true`, it is eligible for AI enrichment and
  semantic embedding, and the prune business rule does not treat it as
  removable

### Requirement: Audio design leaves the product-design category

The system SHALL resolve audio work to `creative` rather than to `design`. A
"Sound Designer" is filed with product and experience designers today for no
reason other than the word "designer" appearing in the title, which puts audio
craft in the facet a UX candidate filters by.

#### Scenario: Audio titles move out of design

- **WHEN** a job titled "Sound Designer" or "Audio Designer" is classified
- **THEN** its category is `creative`, not `design`

### Requirement: Product and visual design keep the design category

The system SHALL leave `motion designer`, `graphic designer`, `visual
designer`, `brand designer`, `web designer` and the product/UX design titles
resolving to `design`. This change adds a home for titles that resolve to
nothing today; it does not re-cut a facet that users already filter by, that
saved searches already reference, and that landing pages already count.

#### Scenario: Motion and graphic design stay put

- **WHEN** a job titled "Motion Designer", "Motion Graphics Designer",
  "Graphic Designer", "Visual Designer" or "Brand Designer" is classified
- **THEN** its category is `design`, and its existing named role is unchanged

#### Scenario: Content and UGC creation are unaffected

- **WHEN** a job titled "Content Creator" or "UGC Creator" is classified
- **THEN** its named role is `content_creator` and its category is unchanged —
  `marketing` for the former, none for the latter, which resolves no category
  today and MUST NOT start resolving to `creative`

### Requirement: A creative alias never steals from a more specific alias

Title matching resolves the longest alias first, and several creative words are
also tool names or qualifiers inside longer titles. The system SHALL order the
new aliases so that a title naming a more specific craft keeps that craft, and
SHALL carry a regression test for each collision the vocabulary creates.

#### Scenario: A design title naming Illustrator stays with design

- **WHEN** a job titled "Graphic Designer (Illustrator, Photoshop)" is
  classified
- **THEN** its category is `design`, because `graphic designer` is the longer
  alias — while a bare "Illustrator" resolves to `creative`

#### Scenario: A bare craft word does not resolve on its own

- **WHEN** a job titled "Audio DSP Engineer" is classified
- **THEN** its category is not `creative`: only the qualified phrases resolve,
  never the bare words "audio", "video", "art" or "sound"

#### Scenario: A qualified engineering title is unaffected

- **WHEN** a job titled "Mechanical Design Engineer" is classified
- **THEN** its category is still `engineering_design`

### Requirement: Named roles cover the media crafts and game development

The system SHALL expose named roles for the media crafts, so a candidate can
filter for the craft rather than for the coarse category. It SHALL additionally
expose named roles for the game-development titles that today collapse into a
bare `design` or `software_engineering` category, WITHOUT introducing a game
category: `game designer`, `level designer` and `narrative designer` keep
`design`, `game developer` keeps `software_engineering`, and `game producer`
keeps the empty category it resolves to today — a named role is emitted whether
or not a category resolves.

#### Scenario: A media craft resolves to its own role

- **WHEN** a job titled "Senior Video Editor" is classified
- **THEN** its roles include `video_editor`

#### Scenario: A game title resolves to a role without changing its category

- **WHEN** a job titled "Game Designer", "Level Designer", "Narrative
  Designer", "Game Producer" or "Game Developer" is classified
- **THEN** its roles include the corresponding named role, and its category is
  the one it resolves to today

### Requirement: The skill dictionary covers the creative toolchain

The system SHALL tag the tools these crafts name — the video editors, the 3D
and compositing suites, the game engines, and the working techniques — so a
creative posting carries skills a candidate can filter by. A token that is also
ordinary English or a term of art in another discipline MUST be gated by
corroboration rather than tagged outright, and a phrase that cannot be gated
MUST be omitted rather than shipped ungated.

#### Scenario: An unambiguous creative tool is tagged

- **WHEN** a description names "DaVinci Resolve", "Final Cut Pro", "Cinema 4D",
  "CapCut", "Godot", "Houdini", "Substance Painter" or "ZBrush"
- **THEN** the job carries the corresponding canonical skill

#### Scenario: An ambiguous creative token needs corroboration

- **WHEN** a backend description says "the UI animation is handled upstream"
  and names no other creative tool
- **THEN** no `animation` skill is tagged

### Requirement: The new category is labelled and selectable

The system SHALL label `creative` on every surface that renders a facet code
and SHALL place it in a group of the web category picker. A category absent
from the picker's section map is generated into the contracts but unreachable
by a user, so labelling alone does not satisfy this requirement.

#### Scenario: The category renders and can be chosen

- **WHEN** a user opens the category filter
- **THEN** `creative` appears under its own "Creative & Media" group with a
  human label, and selecting it filters the job list
