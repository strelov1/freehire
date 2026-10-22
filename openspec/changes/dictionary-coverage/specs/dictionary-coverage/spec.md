## ADDED Requirements

### Requirement: The page lists what the dictionaries hold
The `/dictionary` page MUST be public and MUST show two sections. The Specialization section MUST list every specialization in the client vocabulary (`CATEGORY_GROUPS`), grouped and collapsible, using the filter modal's `CategoryPane` in include-only mode. The Skills section MUST list every skill in the skills glossary catalog, using the filter modal's `SearchSelect` with tech icons. A term MUST be listed whether or not it has open jobs.

#### Scenario: A term with no open jobs is listed
- **WHEN** a specialization or skill in the dictionary has no open jobs
- **THEN** the page still lists it

#### Scenario: An anonymous visitor sees the page
- **WHEN** a visitor who is not signed in opens `/dictionary`
- **THEN** both sections render

### Requirement: Open-job counts are best-effort
The page MUST get its counts from one unfiltered facet call for `category` and `skills`. When that call fails, the page MUST still render every term, without numbers.

#### Scenario: Search is down
- **WHEN** the facet call fails
- **THEN** the page renders both lists and shows no counts

### Requirement: Picked chips link to jobs
When the visitor picks one or more chips, the page MUST show a "Show jobs" link to `/jobs` carrying those filters. With nothing picked, the link MUST NOT appear.

#### Scenario: A picked skill becomes a job search
- **WHEN** the visitor picks a skill chip
- **THEN** a "Show jobs" link to `/jobs` with that skill filter appears

### Requirement: A search with no match offers to add the term
When a section's search matches no listed term, the page MUST show "“term” is not in the dictionary." and an "Add to the list" button in place of "Nothing found". It MUST NOT send a write until the button is clicked. The stored kind MUST be the section's: `specialization` or `skill`.

#### Scenario: A miss waits for the click
- **WHEN** the visitor searches for a term no listed specialization matches
- **THEN** the page shows the add button and no gap is written until it is clicked

#### Scenario: A skill miss is stored as a skill
- **WHEN** a signed-in user clicks "Add to the list" in the Skills section
- **THEN** the new or updated row has kind `skill`

### Requirement: Anonymous add goes through sign-in
An anonymous add MUST be refused with 401 and MUST NOT write a row. The page MUST then open sign-in with the term in the URL (`?specialization=` or `?skill=`), so the visitor returns with the term in that section's search box.

#### Scenario: Anonymous click
- **WHEN** a visitor who is not signed in clicks "Add to the list"
- **THEN** no row is written and the visitor is sent to sign in, returning with the term seeded in the search

### Requirement: The server decides whether a term is known
`POST /api/v1/dictionary/gaps` MUST check the term against the dictionary for its kind. A `specialization` is known when `classify.Parse(term).Category` is not empty. A `skill` is known when `skilltag.Canonicalize` returns a canonical for it. A known term MUST be refused with 409 and MUST NOT change a row. An unknown kind MUST be refused with 400. The page MUST show a 409 as "already in the dictionary under another name".

#### Scenario: An alias the page filter missed
- **WHEN** a signed-in user adds a skill the page found no match for but `skilltag` resolves
- **THEN** the request is refused with 409, no row changes, and the page says it is already in the dictionary under another name

#### Scenario: An unknown kind is rejected
- **WHEN** a request uses a kind other than `specialization` or `skill`
- **THEN** the request is refused with 400 and no row is written

### Requirement: One live row per kind and normalized term
The system MUST keep at most one `pending` or `accepted` row for a `(kind, normalized term)`. The normalized term MUST be the term folded by trimming, lowercasing, mapping `ё` to `е`, spelling `&` as `and`, collapsing every other run of characters that are not letters or digits into a single hyphen, and trimming hyphens. A second write of a `pending` or `accepted` row MUST increment `request_count` and `last_requested_at` and MUST leave `raw_term` and `submitted_by` as the first write set them. When every existing row for that key is `rejected`, the next write MUST insert a new `pending` row and MUST leave the rejected row and its note in place. A term that folds to empty, or that is longer than 80 characters (not bytes) after trim, MUST be refused with 400 and MUST NOT be written. A successful write MUST return that term's `request_count` and `status` only, and the page MUST show it as "Asked N times". No route MUST return the queue.

#### Scenario: A repeated pending term increments
- **WHEN** a signed-in user adds a term whose pending row already exists
- **THEN** `request_count` increases by one, `last_requested_at` moves forward, and `raw_term` stays the first spelling

#### Scenario: An accepted term keeps counting
- **WHEN** a signed-in user adds a term whose live row is `accepted`
- **THEN** `request_count` increases by one and the status stays `accepted`

#### Scenario: A rejected term can be asked again
- **WHEN** a signed-in user adds a term whose only rows are `rejected`
- **THEN** a new `pending` row is inserted and the rejected row remains unchanged

#### Scenario: Two first asks at once both count
- **WHEN** two signed-in users add the same new term at the same moment
- **THEN** both succeed, one live row exists, and its `request_count` is 2

#### Scenario: Two spellings share a key
- **WHEN** `Back-End` and `back end` are added as the same kind
- **THEN** they update one live row

#### Scenario: An empty or oversized term is refused
- **WHEN** the term folds to empty, or is longer than 80 characters after trim
- **THEN** the request is refused with 400 and no row is written

### Requirement: A user can record at most twenty gaps a day
A signed-in user MUST be allowed at most 20 successful gap writes per UTC day, counted in the same transaction as the write. An increment MUST count as one write. The 21st write of that day MUST be refused with 429 and MUST NOT change a gap row. The page MUST show it as a daily limit.

#### Scenario: The twenty-first write is refused
- **WHEN** a signed-in user has already recorded 20 gaps during the current UTC day and adds another
- **THEN** the write is refused with 429, the gap row is unchanged, and the page says the daily limit is reached

### Requirement: The job filter points at the page
The job filter modal's Role pane and Skills pane MUST each end with a link to `/dictionary` ("Can't find your role? Help us recognize it", "Can't find your skill? Help us recognize it"); the skills link MUST open the Skills tab. When a specialization or skill search in the modal matches nothing, the pane MUST say so and link to `/dictionary` with the term as `?specialization=` or `?skill=`.

#### Scenario: A filter search that finds nothing hands its term over
- **WHEN** a visitor searches the filter's skills for a term that matches no option and follows the link
- **THEN** `/dictionary` opens on the Skills tab with that term in the search box

### Requirement: The page is linked from the footer
The site footer Product group MUST include a link to `/dictionary` immediately after the Skills glossary link. The page MUST be in English.

#### Scenario: The footer points at the page
- **WHEN** the footer Product group is rendered
- **THEN** a Dictionary link to `/dictionary` follows the Skills glossary link
