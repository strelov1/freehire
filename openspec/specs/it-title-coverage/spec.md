# it-title-coverage Specification

## Purpose

The IT titles this catalogue carries in volume that the category dictionary had
no word for at all: the Russian software and administration vocabulary, the
Systems Engineer family and the industrial namesakes it must not sweep in, the
vendor-platform titles, and the infrastructure and end-user-IT tail — plus the
named roles they expose.

It exists because of one measurement. Of 1 971 625 open postings in the 60 000
largest title groups, 935 604 reached the search index with an empty `roles`
array, and **every one of them had an empty category** — not a single posting
existed where the category resolved and the role did not. `roletag` derives a
bare role from the category, so category coverage IS role coverage, and role
aliases buy nothing while the category is empty.
## Requirements
### Requirement: The Russian software vocabulary resolves

The system SHALL resolve the Russian titles for software work and system
administration, which carry no English alias and therefore resolve to nothing
today. `программист` and `разработчик` MUST resolve as bare tokens, because the
qualified spellings the catalogue carries put the technology first
("Java-разработчик", "Python-разработчик") and a hyphen is a word boundary, so
no qualified alias can stand in for the bare one.

#### Scenario: A Russian software title resolves to software engineering

- **WHEN** a job titled "Программист", "Инженер-программист",
  "Техник-программист", "Разработчик", "Java-разработчик" or
  "Python-разработчик" is classified
- **THEN** its category is `software_engineering`

#### Scenario: A Russian administration title resolves to its discipline

- **WHEN** a job titled "Системный администратор" or "Администратор баз данных"
  is classified
- **THEN** its category is `devops`

#### Scenario: A Russian network title resolves to networking

- **WHEN** a job titled "Сетевой администратор" is classified
- **THEN** its category is `network_engineering`

### Requirement: The Systems Engineer family resolves, and its lookalikes do not

`Systems Engineer` is the largest unresolved IT title in the catalogue (1440
open postings for the exact spelling alone), but the same words name electrical,
control and manufacturing work. The system SHALL resolve the bare and
IT-qualified spellings to a technical category, and SHALL declare the non-IT
qualified spellings BLIND (the sentinel canonical) ABOVE the bare alias, so they
keep resolving to no category rather than being swept into software by the
shorter alias below them.

Blindness rather than a category is deliberate: those titles belong to an
industrial taxonomy this change does not introduce, and the sentinel is what
keeps them from silently landing somewhere wrong in the meantime.

#### Scenario: The bare and IT-qualified spellings resolve

- **WHEN** a job titled "Systems Engineer", "System Engineer", "Systems
  Engineer II", "IT Systems Engineer" or "Software Systems Engineer" is
  classified
- **THEN** its category is `software_engineering`

#### Scenario: An infrastructure-qualified spelling resolves to infrastructure

- **WHEN** a job titled "Linux Systems Engineer" is classified
- **THEN** its category is `devops`

#### Scenario: A security-qualified spelling resolves to security

- **WHEN** a job titled "Cyber Systems Engineer" is classified
- **THEN** its category is `security`

#### Scenario: The non-IT lookalikes stay unresolved

- **WHEN** a job titled "Control Systems Engineer", "Power Systems Engineer",
  "Electrical Systems Engineer" or "Quality Systems Engineer" is classified
- **THEN** it resolves to NO category — the bare "systems engineer" alias
  declared below them must not claim it

### Requirement: Vendor-platform titles resolve to their discipline

A title naming an enterprise platform states its discipline as surely as a
language does. The system SHALL resolve the platform titles the catalogue
carries in volume: the ServiceNow family, the Salesforce family beyond the
already-resolved Developer, Oracle DBA, SharePoint Administrator, Mainframe
Developer and Tableau Developer.

#### Scenario: A platform development title resolves to software engineering

- **WHEN** a job titled "ServiceNow Developer", "ServiceNow Engineer",
  "Salesforce Administrator", "Salesforce Engineer", "Salesforce Consultant" or
  "Mainframe Developer" is classified
- **THEN** its category is `software_engineering`

#### Scenario: A platform administration title resolves to infrastructure

- **WHEN** a job titled "Oracle DBA", "SharePoint Administrator" or "ServiceNow
  Administrator" is classified
- **THEN** its category is `devops`

#### Scenario: A reporting-platform title resolves to analytics

- **WHEN** a job titled "Tableau Developer" is classified
- **THEN** its category is `data_analytics`

### Requirement: The infrastructure and IT-support tail resolves

The system SHALL resolve the operational titles the catalogue carries that name
infrastructure or end-user IT: the data-centre titles, the release and cloud
operations titles, the network operations titles, and the IT
specialist/technician titles.

#### Scenario: Data-centre and cloud operations resolve to infrastructure

- **WHEN** a job titled "Data Center Technician", "Data Center Engineer",
  "Release Engineer", "Cloud Operations Engineer", "Cloud Migration Engineer"
  or "Network Operations Engineer" is classified
- **THEN** its category is `devops`

#### Scenario: Network field titles resolve to networking

- **WHEN** a job titled "Network Specialist" or "Network Technician" is
  classified
- **THEN** its category is `network_engineering`

#### Scenario: End-user IT resolves to IT support

- **WHEN** a job titled "IT Specialist" or "IT Technician" is classified
- **THEN** its category is `it_support`, the same category "IT Support Specialist"
  already resolves to

#### Scenario: The integration family resolves to software engineering

- **WHEN** a job titled "Integration Engineer", "Systems Integration Engineer",
  "Software Integration Engineer", "Data Integration Engineer" or "Cloud
  Integration Engineer" is classified
- **THEN** its category is `software_engineering`

### Requirement: The new aliases never steal from a title that already resolves

Several of these words occur inside titles from other disciplines, and the
title table resolves in declaration order. The system SHALL carry a regression
test for each collision the vocabulary creates, naming the title that must keep
its existing category.

#### Scenario: Retail and trades titles are untouched

- **WHEN** a job titled "Parts Counterperson", "Parts Interpreter", "Pit
  Technician" or "SAP Operations Clerk Part Time Day" is classified
- **THEN** it resolves to no category, exactly as it does today

#### Scenario: Customer-facing engineering keeps its category

- **WHEN** a job titled "Sales Engineer" or "Support Engineer" is classified
- **THEN** its category is `solutions_engineering` and `support` respectively,
  unchanged

### Requirement: The platform and systems crafts expose named roles

The system SHALL name the roles these titles describe, so a candidate filters
by the craft rather than by the coarse category: `salesforce_developer`,
`sap_developer`, `servicenow_developer` and `systems_engineer`.

`systems_administrator` SHALL additionally resolve from the SINGULAR spelling.
It exists today but is reachable only through "Systems Administrator", so
"System Administrator", "Sysadmin", "Linux System Administrator" and the
Russian "Системный администратор" get the category and no role — a gap invisible
from the role catalogue, which lists the slug as covered.

#### Scenario: A platform title resolves to its named role

- **WHEN** a job titled "Salesforce Developer", "SAP ABAP Developer" or
  "ServiceNow Developer" is classified
- **THEN** its roles include `salesforce_developer`, `sap_developer` and
  `servicenow_developer` respectively

#### Scenario: The singular administrator spelling resolves to the role

- **WHEN** a job titled "System Administrator", "Sysadmin", "Linux System
  Administrator" or "Системный администратор" is classified
- **THEN** its roles include `systems_administrator`

#### Scenario: The systems engineer craft is pickable

- **WHEN** a job titled "Systems Engineer" is classified
- **THEN** its roles include `systems_engineer`

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

