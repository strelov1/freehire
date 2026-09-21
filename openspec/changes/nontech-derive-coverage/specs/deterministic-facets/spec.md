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

<!--
Deliberately not a requirement yet: whether "work from home" also needs a
`travelPerkPhrases` guard (the same treatment "work from anywhere" carries
for freehire#2696, since the phrase also appears in bounded benefit-list
prose). The design record requires deciding that from a sample of live
descriptions, not by assumption — see design.md's Migration Plan. Until that
sample is pulled and the decision made, "work from home" resolves `remote`
unconditionally like every other phrase in the list above.
-->
