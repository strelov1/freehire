## MODIFIED Requirements

### Requirement: The role fingerprint ignores a location-bearing title suffix

The `role_fingerprint` that keys a role cluster SHALL normalize the title by
stripping a single trailing separator clause — the text after the last ` , `,
` | `, ` @ `, or space-delimited ` - ` / ` — ` / ` – ` — before hashing, so a role
whose only difference is a city (or other qualifier) appended to the title
resolves to the same fingerprint as its siblings. The strip SHALL remove only a
trailing clause (never a prefix, so a seniority grade like `Senior …` is
preserved) and SHALL leave the title unchanged when stripping would drop it below
two words.

Before the case/whitespace fold, the title and the description SHALL be reduced to
their **visible text** — HTML tags stripped and HTML entities decoded — so two
postings whose rendered text is identical share a fingerprint even when their
markup differs (a stray tag, a different entity encoding, or a different wrapper
structure from another source). The description SHALL remain part of the
fingerprint, so two postings collapse only when both the stripped title AND the
visible description match; postings whose visible text differs still resolve to
different fingerprints.

#### Scenario: Per-city title variants share one fingerprint

- **WHEN** a company posts one role in several cities and each posting appends the
  city to the title (e.g. `"… Engineer, Krakau"`, `"… Engineer, Wien"`) with an
  identical description
- **THEN** all the postings resolve to the same `role_fingerprint` and collapse to
  one canonical card

#### Scenario: Markup-only differences share one fingerprint

- **WHEN** two postings have the same company, the same stripped title, and the
  same visible description text, but their description HTML differs only in markup
  (e.g. one has an extra `<br>`, or encodes `&` as `&amp;`)
- **THEN** they resolve to the same `role_fingerprint` and collapse to one
  canonical card

#### Scenario: Distinct roles with different descriptions stay separate

- **WHEN** two postings share a stripped title but carry different visible
  descriptions (e.g. two engineering specialties, or a city-specific legal clause
  present in one and absent in the other)
- **THEN** they resolve to different `role_fingerprint`s and are not collapsed

#### Scenario: A seniority prefix is never stripped

- **WHEN** a title carries a leading grade (e.g. `"Senior Software Engineer"`)
- **THEN** the grade is retained in the fingerprint, so it does not collapse into
  the ungraded role
