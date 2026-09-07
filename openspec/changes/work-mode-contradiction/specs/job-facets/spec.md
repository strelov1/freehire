## MODIFIED Requirements

### Requirement: A description may contradict a remote work mode

The work-mode derivation SHALL treat an explicit denial of remote work in a posting's
description as OVERRULING a `remote` work mode, whichever source produced it — the
structured ATS signal, the parsed location marker, or the description's own gap-filling
phrase list.

The denial dictionary SHALL be separate from the gap-filling phrase list, and SHALL admit
only sentences whose purpose is to deny remote work. A denial followed within 60 characters
by a qualifier that scopes it to something other than this posting's arrangement — a trial
period, or a role the posting might lead to — SHALL NOT count, and the scan SHALL continue
rather than concluding the posting is remote.

The override SHALL apply only to a `remote` result and SHALL produce `onsite`. A `hybrid`
result SHALL be left unchanged.

Matching SHALL be performed against the description with HTML markup and entities folded
away, so where an editor placed a tag does not decide the answer.

#### Scenario: An employer's own body overrules their ATS location bucket

- **WHEN** a posting's location reads `US, TX, Remote` and its description states
  "This position is 100% on-site based at either our Dallas or Houston facility"
- **THEN** the derived work mode is `onsite`

#### Scenario: A structured remote flag is overruled too

- **WHEN** an adapter supplies a structured work mode of `remote` and the description
  states "Onsite - not a remote position"
- **THEN** the derived work mode is `onsite`

#### Scenario: A trial period is not a denial

- **WHEN** a description states "fully on-site for the first 90 days at the headquarters"
- **THEN** the derived work mode is unchanged

#### Scenario: A denial says nothing against hybrid

- **WHEN** a posting's work mode resolves to `hybrid` and its description states
  "This is not a remote position; you will be in the office three days a week"
- **THEN** the derived work mode remains `hybrid`

#### Scenario: Markup inside the phrase does not hide it

- **WHEN** a description states "This is not a <strong>REMOTE POSITION</strong>"
- **THEN** the denial is recognised
