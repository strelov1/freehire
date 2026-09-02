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

All four spellings move together. Leaving any behind scatters one craft across
three categories: the "…Design Engineer" forms fall through the bare "design
engineer" alias into draughting unless they are declared above it.

Bare "Audio Engineer" and "Sound Engineer" SHALL NOT resolve. They name
broadcast, live sound and AV integration as often as this craft, and a
field-service AV engineer labelled "Sound Designer" is worse than an unnamed
row.

#### Scenario: Audio titles move out of design

- **WHEN** a job titled "Sound Designer", "Audio Designer", "Sound Design
  Engineer" or "Audio Design Engineer" is classified
- **THEN** its category is `creative` — not `design`, and not
  `engineering_design` for the two engineer spellings

#### Scenario: The broadcast spellings stay unresolved

- **WHEN** a job titled "Audio Engineer" or "Field Service Engineer - Audio
  Engineer" is classified
- **THEN** it resolves to no category and to no media-craft role

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

### Requirement: A creative alias never steals from a working facet

The title table resolves in DECLARATION ORDER — first whole-word match wins —
and every media craft is also a tool or a second hat named inside someone
else's title. The system SHALL declare the craft aliases LAST, after every
other category, so a title resolves to a craft only when it names no other
discipline. The audio spellings are the stated exception, declared where they
must be to work at all. Each collision the vocabulary creates SHALL carry a
regression test naming the title it must NOT take.

The accepted cost SHALL be that a title whose craft is qualified by another
discipline resolves to that discipline: a "Social Media Video Editor" is
`marketing`. The posting stays findable on a facet already correct for it,
whereas a stolen row is a regression.

#### Scenario: A design title naming a craft or a tool keeps design

- **WHEN** a job titled "Graphic Designer (Illustrator, Photoshop)", "Graphic
  Designer & Photographer" or "Junior Motion Designer / Animator" is classified
- **THEN** its category is `design` and its role is the design role — while a
  bare "Illustrator", "Photographer" or "Animator" resolves to `creative`

#### Scenario: A marketing title naming a craft or a tool keeps marketing

- **WHEN** a job titled "Marketing Specialist (Photoshop, Illustrator)",
  "Social Media Manager (Canva, Illustrator)" or "Social Media Video Editor" is
  classified
- **THEN** its category is `marketing`

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

Each craft SHALL keep its own slug where the crafts are genuinely different
jobs — a VFX artist rendered as "Storyboard Artist", or a 2D artist as "3D
Artist", is wrong data on a facet a candidate filters by. Seats on one pipeline
MAY share a slug: character and environment art fold into `3d_artist`.

#### Scenario: A media craft resolves to its own role

- **WHEN** a job titled "Senior Video Editor" is classified
- **THEN** its roles include `video_editor`

#### Scenario: Distinct crafts keep distinct slugs

- **WHEN** a job titled "VFX Artist" or "2D Artist" is classified
- **THEN** its roles include `vfx_artist` and `2d_artist` respectively, while
  "Character Artist" and "Environment Artist" resolve to `3d_artist`

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

Which tier an entry lands in is decided by WHICH TABLE it is declared in:
`ambiguousWords` reaches the word pass only, and `nonCorroboratingPhrases` the
phrase pass only. A single token declared as a phrase is therefore ungated no
matter what `ambiguousWords` says, and a craft name declared as a word cannot
be stopped from vouching.

Two products that are two jobs SHALL be two canonicals: Substance 3D Painter
(texture painting) and Substance 3D Designer (procedural material authoring)
are not spellings of each other, and folding them would have the public
glossary render one as an alias of the other.

#### Scenario: An unambiguous creative tool is tagged

- **WHEN** a description names "DaVinci Resolve", "Final Cut Pro", "Cinema 4D",
  "CapCut", "Godot", "Substance Painter" or "ZBrush"
- **THEN** the job carries the corresponding canonical skill

#### Scenario: The two Substance products stay apart

- **WHEN** a description names "Substance Designer"
- **THEN** the job carries `substance-designer` and NOT `substance-painter`

#### Scenario: A craft name tags but does not vouch

- **WHEN** a marketing posting lists "video editing" among its duties beside
  the word "Spring", or a product posting says "build storyboards, sketch out
  flows"
- **THEN** the craft is tagged and the gated words beside it are not: no
  `spring`, no `sketch`

#### Scenario: An ambiguous product name needs corroboration

- **WHEN** an events posting advertises "a Houdini-style escape act", or a
  pathology posting says "interpret C4d staining", and neither names another
  tool
- **THEN** no `houdini` and no `cinema-4d` skill is tagged — while an FX
  posting naming Houdini beside Substance Painter carries both

#### Scenario: A token the gate cannot save is omitted

- **WHEN** a frontend posting says "React, CSS animation and Tailwind", or a
  platform posting says "nuke the cache and redeploy via Terraform"
- **THEN** no `animation` and no `nuke` skill is tagged, because neither is in
  the dictionary at all: the corroboration gate is lifted by ANY strong skill,
  and both of these collide with prose that always sits beside one

### Requirement: The new category is labelled and selectable

The system SHALL label `creative` on every surface that renders a facet code
and SHALL place it in a group of the web category picker. A category absent
from the picker's section map is generated into the contracts but unreachable
by a user, so labelling alone does not satisfy this requirement.

#### Scenario: The category renders and can be chosen

- **WHEN** a user opens the category filter
- **THEN** `creative` appears labelled "Creative & Media" in the picker's
  craft section — renamed from "Design" to "Design & Creative", since that
  section now holds video, audio and photography beside the two design
  categories — and selecting it filters the job list
