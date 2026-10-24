## ADDED Requirements

### Requirement: Field operators in the search box

The header search box SHALL recognise `title:`, `company:` and `source:` operators, each
followed by one word or a double-quoted phrase, and SHALL treat any other `word:` as
ordinary text.

#### Scenario: Operators become filters on Enter
- **WHEN** the visitor types `company:Google source:greenhouse rust` and presses Enter
- **THEN** the `company_slug` facet holds `google`, the `source` facet holds `greenhouse`,
  `q` is `rust`, and the box is cleared

#### Scenario: Quoted value
- **WHEN** the visitor commits `company:"Goldman Sachs"`
- **THEN** the company facet holds Goldman Sachs' slug

#### Scenario: Unknown key is text
- **WHEN** the visitor commits `c++: senior`
- **THEN** `q` is `c++: senior` and no facet changes

#### Scenario: Title operator restricts the search to titles
- **WHEN** the visitor commits `title:"Backend Engineer" rust`
- **THEN** `q` is `"Backend Engineer rust"` and `q_fields` is `title`

#### Scenario: Unresolved value stops the commit
- **WHEN** the visitor commits `company:Nosuchco java` and no company is named `Nosuchco`
- **THEN** no filter and no query change, the box keeps its text, the caret moves to the
  end of `Nosuchco`, and the panel says no company matches it

### Requirement: Contextual suggestions after an operator

While the caret is inside an operator's value, the dropdown SHALL offer only suggestions of
that operator's kind, and picking one SHALL replace the token with the canonical value.

#### Scenario: Company suggestions
- **WHEN** the visitor types `java company:goo`
- **THEN** the dropdown lists companies whose name starts with `goo`, busiest first, and no
  other sections
- **AND** picking Google turns the box text into `java company:Google `

#### Scenario: Source suggestions
- **WHEN** the visitor types `source:gre`
- **THEN** the dropdown lists sources whose key or label starts with `gre`

#### Scenario: Plain text keeps today's dropdown
- **WHEN** the caret is not inside an operator value
- **THEN** the dropdown shows the usual sections, fetched with the operators stripped from
  the text
