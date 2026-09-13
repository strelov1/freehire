## ADDED Requirements

### Requirement: German fused-compound IT titles resolve

German compounds a title's role words into one unbroken word with no
separator (`Systemadministrator`, `Fachinformatiker`), so the word-boundary
matcher that resolves a hyphenated Russian title or a spaced English one can
never reach the German spelling unless the fused form is its own alias. The
system SHALL resolve the following fused-compound German IT titles bare, to
the same category their already-covered spaced/English counterpart resolves
to: `Systemadministrator` (as `system administrator` already does, and the
bare alias already reaches an `IT-`/`IT `-prefixed spelling because the
preceding hyphen or space is itself a word boundary), `Netzwerkadministrator`
(as `network administrator` already does), `Datenbankadministrator` (as
`database administrator` already does), `Netzwerktechniker` (as `network
technician` already does), `Softwaretester` (as bare `tester` already does),
and `Anwendungsentwickler` (joining the existing German `entwickler`
cluster).

It SHALL also resolve `Fachinformatiker` — the German formal IT-specialist
title and apprenticeship — bare, and additionally resolve its `für
Systemintegration`/`Systemintegration` qualifier to `devops` and its `für
Anwendungsentwicklung`/`Anwendungsentwicklung` qualifier to
`software_engineering` when that qualifier is adjacent to the word (the bare
alias, declared after the qualified ones, is the fallback for every other
phrasing, since `Fachinformatiker` never names a non-IT role).

`Systemtechniker` and `Systemelektroniker` also name non-IT disciplines
(mechanical/electrical "Systemtechniker Elektrotechnik", physical-security
"Systemtechniker Sicherheitstechnik") — the same cross-domain trap this spec
already documents for the bare "Systems Engineer" family. The system SHALL
resolve ONLY the `IT`-qualified spellings (`IT Systemtechniker`,
`IT-Systemtechniker`, `IT Systemelektroniker`, `IT-Systemelektroniker`) and
MUST NOT resolve the bare word.

`SPS-Programmierer` (PLC/industrial-controller programming) is explicitly
OUT of scope: it is already deliberately excluded from resolving to a
software category, on the same reasoning the dictionary already applies to
"CNC Programmer".

#### Scenario: German IT administrator and technician titles resolve

- **WHEN** a job titled "Systemadministrator (m/w/d)", "IT-Systemadministrator
  (m/w/d)", "IT Systemadministrator (m/w/d)", "Netzwerkadministrator (m/w/d)"
  or "Datenbankadministrator (m/w/d)" is classified
- **THEN** its category is `devops` for the system/database administrator
  spellings and `network_engineering` for the network administrator spelling

#### Scenario: German network technician and software tester titles resolve

- **WHEN** a job titled "Netzwerktechniker (m/w/d)", "IT-Netzwerktechniker
  (m/w/d)" or "Softwaretester (m/w/d)" is classified
- **THEN** its category is `network_engineering` for the network technician
  spellings and `qa` for the software tester spelling

#### Scenario: Anwendungsentwickler resolves

- **WHEN** a job titled "Anwendungsentwickler (m/w/d)" or "Inhouse
  Anwendungsentwickler (m/w/d)" is classified
- **THEN** its category is `software_engineering`

#### Scenario: Fachinformatiker resolves bare and by qualifier

- **WHEN** a job titled "Fachinformatiker (m/w/d)", "Fachinformatiker
  Systemintegration (m/w/d)", "Fachinformatiker für Systemintegration
  (m/w/d)", "Fachinformatiker Anwendungsentwicklung (m/w/d)" or
  "Fachinformatiker für Anwendungsentwicklung (m/w/d)" is classified
- **THEN** its category is `devops` for the bare and Systemintegration
  spellings and `software_engineering` for the Anwendungsentwicklung spelling

#### Scenario: IT-qualified Systemtechniker and Systemelektroniker resolve, the bare word does not

- **WHEN** a job titled "IT Systemtechniker (m/w/d)", "IT-Systemtechniker
  (m/w/d)", "IT Systemelektroniker (m/w/d)" or "IT-Systemelektroniker (m/w/d)"
  is classified
- **THEN** its category is `devops`

#### Scenario: Non-IT Systemtechniker stays unresolved

- **WHEN** a job titled "Systemtechniker (m/w/d) Elektrotechnik" or
  "Systemtechniker Sicherheitstechnik (m/w/d)" is classified
- **THEN** it resolves to NO category — the unqualified word must not be
  swept into a technical category

#### Scenario: SPS-Programmierer is out of scope and stays unresolved

- **WHEN** a job titled "SPS-Programmierer (m/w/d)" is classified
- **THEN** it resolves to NO category, exactly as it does today, matching the
  existing "CNC Programmer" exclusion this dictionary already applies
