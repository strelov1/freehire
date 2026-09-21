## ADDED Requirements

### Requirement: The home-working register resolves to remote work_mode

`descriptionWorkModePhrases` SHALL recognize the home-working register
administrative and virtual-assistant postings actually use, not only the
corporate register already covered: "work from home", "work-from-home",
"home-based", "home based", "telecommute", "virtual position". This source
remains the lowest-priority `work_mode` signal — it only fills a value the
structured ATS signal and the parsed location marker left empty, and never
overwrites either.

#### Scenario: Home-working phrase fills an empty work_mode

- **WHEN** a job's structured source and parsed location leave `work_mode`
  empty, and the description states "This is a work-from-home position"
- **THEN** the derived `work_mode` is `remote`

#### Scenario: A structured signal is never overwritten by the phrase

- **WHEN** a job's structured source sets `work_mode=hybrid`, and the
  description also states "occasional telecommute days available"
- **THEN** the derived `work_mode` stays `hybrid`

### Requirement: A bounded travel-perk phrase does not assert remote work

"work from home" SHALL be evaluated as a `travelPerkPhrases` guard candidate,
the same treatment already applied to "work from anywhere" (freehire#2696):
when the phrase appears inside a bounded perk/benefit clause rather than as a
statement of work arrangement, it SHALL NOT resolve `work_mode` to `remote`.

#### Scenario: A benefit-list mention does not assert remote work

- **WHEN** a description states "flexible PTO, occasional work from home days,
  and a home office stipend" as part of a benefits list, with no other
  work-arrangement statement
- **THEN** the phrase does not resolve `work_mode` to `remote`
