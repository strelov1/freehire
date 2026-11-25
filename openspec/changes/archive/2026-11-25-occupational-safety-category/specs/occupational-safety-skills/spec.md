## ADDED Requirements

### Requirement: The occupational-safety profession has a deterministic skill vocabulary

The system SHALL carry the occupational-safety skill vocabulary in the deterministic
skill dictionary, because nothing else can tag this population: these postings derive
`is_tech = false`, the enrichment enqueue gate reads `is_tech IS TRUE`, so the LLM never
sees them and there is no later pass that fills a gap here.

The vocabulary SHALL cover at minimum the terms measured in the profession's own live
descriptions: `osha`, `iso-14001`, `iso-45001`, `emergency-response`,
`personal-protective-equipment`, `root-cause-analysis`, `environmental-compliance`,
`risk-assessment`, `incident-investigation`, `safety-management-system`,
`industrial-hygiene`, `epa`, `corrective-action`, `waste-management`, `first-aid`,
`hazardous-waste`, `toolbox-talks`, `safety-audit`, `hazard-identification`,
`contractor-safety`, `lockout-tagout`, `confined-space`, `nfpa`, `fall-protection`,
`machine-guarding`, `hot-work`, `near-miss-reporting`, `process-safety-management`,
`hazop`, `permit-to-work`, `job-safety-analysis`, `rcra`, `behavior-based-safety` and
`working-at-height`, together with the EHS platforms this market names — `enablon`,
`intelex`, `velocityehs`, `sphera` and `cority`.

Terms SHALL be admitted on measured evidence from live descriptions of this population,
not from a written-down impression of the work. A list written from what a job sounds
like can only be tested against itself.

#### Scenario: A safety posting carries safety skills

- **WHEN** an EHS posting whose description names OSHA, ISO 45001 and incident
  investigation is tagged
- **THEN** its skills include `osha`, `iso-45001` and `incident-investigation`

#### Scenario: The vocabulary reaches a population the LLM never sees

- **WHEN** a posting resolves `occupational_safety` and therefore derives
  `is_tech = false`
- **THEN** it is never enqueued for LLM enrichment, and every skill it carries came from
  the deterministic dictionary

### Requirement: HSE credentials live in the certification dictionary

The system SHALL place the profession's credentials — `nebosh`, `iosh`, `csp`
(Certified Safety Professional), `cih` (Certified Industrial Hygienist), `chmm`
(Certified Hazardous Materials Manager) and `hazwoper` — in the certification
dictionary alongside PMP, CISSP and the cloud certificates, rather than in the skill
dictionary. They are credentials of the same kind, and the skill dictionary names
skills.

#### Scenario: A credential resolves as a certification

- **WHEN** a description or CV names NEBOSH, IOSH or HAZWOPER
- **THEN** it resolves as a certification, not as a skill tag

### Requirement: Ambiguous HSE acronyms are category-scoped or excluded

A three-letter acronym that names something else in this catalogue MUST NOT resolve on
general job text. The system SHALL resolve `BBS` to Behaviour-Based Safety only when the
caller supplies the `occupational_safety` category, through the existing category-scoped
acronym mechanism.

`CSP`, `CIH` and `CHMM` SHALL NOT be admitted to that mechanism. An earlier draft of this
requirement asked for them, and the skill dictionary's own invariant refused: a
category-scoped acronym must resolve to a canonical that ALREADY EXISTS there, because an
acronym is another alias and never a new facet value. All three name credentials rather
than skills, so they belong to `internal/dict/certification` beside PMP and CISSP — where
the design had already placed them for an unrelated reason, and the two arguments agree.

The consequence is stated rather than glossed: the certification dictionary resolves
CV-claimed tokens, not description text, and it is not category-scoped. So these three do
not resolve from a job description at all, under any category. That is the intended
behaviour and not a gap — a posting that lists "CSP required" is stating a requirement,
and the facet this vocabulary feeds describes the posting's skills.

`PSM` SHALL keep its existing `professional-scrum-master` meaning scoped to
`project_management`. The category-scoped acronym table holds one canonical per key, and
Process Safety Management SHALL therefore be admitted only as the spelled-out phrase.
The measured cost is nil — acronym and phrase each appear in 2% of the corpus — and
widening the table's shape to hold a canonical per category would touch every existing
entry for one term.

Bare `ASP` and bare `DOT` SHALL NOT be admitted in any form. Associate Safety
Professional collides with ASP.NET and Department of Transportation collides with the
English word; a corpus probe attributed 17% of postings to `ASP`, and that was the
prefix of "aspects" and "aspiring".

#### Scenario: A scoped acronym resolves inside the category

- **WHEN** a posting already classified `occupational_safety` names BBS in its
  description
- **THEN** it resolves Behaviour-Based Safety

#### Scenario: The same acronym does not resolve outside the category

- **WHEN** a posting classified `software_engineering` names BBS in its description
- **THEN** it does not resolve Behaviour-Based Safety — there it is a Bulletin Board
  System

#### Scenario: The credential acronyms resolve no skill from a description

- **WHEN** a posting classified `occupational_safety` names CSP, CIH or CHMM in its
  description
- **THEN** no skill resolves from them, because they name credentials and live in the
  certification dictionary instead

#### Scenario: PSM keeps its existing meaning

- **WHEN** a posting classified `project_management` names PSM
- **THEN** it resolves `professional-scrum-master`, unchanged by this vocabulary

#### Scenario: Process safety management resolves spelled out

- **WHEN** a posting names "process safety management" in its description
- **THEN** it resolves `process-safety-management`

#### Scenario: The rejected acronyms resolve nothing

- **WHEN** a posting's text contains "aspects", "aspiring" or the word "dot"
- **THEN** no occupational-safety skill or certification resolves from it

