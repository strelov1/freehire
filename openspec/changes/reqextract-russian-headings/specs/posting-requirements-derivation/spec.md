## ADDED Requirements

### Requirement: The heading vocabulary recognizes more than one language

The controlled heading vocabulary SHALL recognize headings written in more than one
language, matched after the same transliteration the vocabulary already applies to
diacritics — a non-Latin heading is folded to its transliterated form and matched
against the vocabulary in that spelling, the same way an accented Latin heading already
is. Widening the vocabulary to a new language SHALL be additive: entries for one
language SHALL NOT change whether a heading in another language matches.

A vocabulary entry for a new language SHALL be added only after being measured against
real postings that use it, not derived from translation alone — a phrase that
transliterates correctly but is never actually used as a heading earns nothing.

#### Scenario: A non-Latin-script heading opens a requirements section

- **WHEN** a posting's description carries a heading written in a language the
  vocabulary recognizes, in that language's own script, followed by a list
- **THEN** the list's items are derived as requirements, the same as for an English
  heading

#### Scenario: A recognized non-English heading with no list still yields nothing

- **WHEN** a posting's description carries a heading the vocabulary recognizes, in
  any recognized language, but the section that follows is not a list the derivation
  can read
- **THEN** the description yields no entries for that section — recognizing the
  heading does not relax the requirement that a list follow it
