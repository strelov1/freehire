## ADDED Requirements

### Requirement: The AI search modal is a describe → plan → results flow

The AI search modal, opened from "Describe with AI" in the filter modal, SHALL present three steps in order — **Describe**, **Search plan**, **Results** — with a step indicator naming the current one. Describe SHALL offer a free-text description, an optional CV upload (PDF) and example searches. Submitting SHALL move to Search plan; running the plan SHALL move to Results. Closing the modal (close button, backdrop, Escape) SHALL leave the job list's filters untouched unless the plan was applied.

#### Scenario: Submitting a description opens the plan step

- **WHEN** a visitor types "Product designer in Brazil" and submits
- **THEN** the modal shows the Search plan step, the visitor's message, and the agent's plan built from it

#### Scenario: Choosing an example search submits it

- **WHEN** a visitor clicks an example search on the Describe step
- **THEN** that example is submitted exactly as if the visitor had typed it

#### Scenario: Closing without applying changes nothing

- **WHEN** a visitor builds and runs a plan, then closes the modal without choosing Show N jobs
- **THEN** the job list's filters are unchanged

### Requirement: The plan states the filters it will apply

Each plan the agent proposes SHALL be shown as the model's one-sentence summary together with preview chips of every value it will apply — included facet values, excluded values struck through, and the query, salary floor, freshness, experience ceiling and visa sponsorship when present — labelled exactly as the filter controls label them. Values the interpretation could not place SHALL be listed as not recognised. The latest plan SHALL carry a card offering **Run search** and **Keep chatting**, which SHALL remain available until a newer plan replaces it.

#### Scenario: Preview chips name the applied filters

- **WHEN** the agent proposes a plan with category design and country Brazil
- **THEN** the plan shows the summary and chips for Design and Brazil

#### Scenario: An exclusion is shown struck through

- **WHEN** a plan excludes the United States
- **THEN** its United States chip is drawn struck through

#### Scenario: Unrecognised words are reported

- **WHEN** the interpretation returns unresolved values
- **THEN** the plan lists them as not recognised

#### Scenario: Keep chatting keeps the plan runnable

- **WHEN** the visitor chooses Keep chatting
- **THEN** the agent asks what to change, the message field is focused, and Run search on the current plan is still available

### Requirement: Refinement rebuilds the plan in chat

A message sent from the chat SHALL be interpreted together with the current plan and SHALL replace it with a complete new plan. The message field SHALL clear when a message is accepted; a message sent while the agent is still working SHALL be refused and SHALL stay in the field. While the agent works, the chat SHALL show the assistant's thinking indicator. A refinement sent from the Results step SHALL re-run the overview for the new plan without asking again.

#### Scenario: A refinement replaces the plan

- **WHEN** the plan is "Product designer roles in Brazil" and the visitor sends "remote only"
- **THEN** a new plan is shown that keeps Brazil and adds remote work mode

#### Scenario: The field clears on send

- **WHEN** the visitor sends a message while the agent is idle
- **THEN** the message appears in the chat and the field is empty

#### Scenario: Refining from Results re-runs the overview

- **WHEN** the visitor sends a refinement from the Results step
- **THEN** the overview recomputes for the new plan

### Requirement: The agent answers honestly when it cannot plan

When the interpretation resolves nothing, the agent SHALL say it could not build a search and suggest naming a role, a skill or a place, and SHALL offer to start over; it SHALL NOT show a plan that would select the whole catalogue. When the server refuses for plan reasons, the agent SHALL say so with the link to plans. Any other failure SHALL be stated in the chat, and the modal SHALL leave its working state.

#### Scenario: An empty interpretation is not a plan

- **WHEN** the visitor sends "test" and the interpretation is empty
- **THEN** the agent explains it could not build a search, no Run search card is shown, and Start over returns to Describe

#### Scenario: A failed run does not hang

- **WHEN** the overview's request fails
- **THEN** the chat shows the error and the thinking indicator stops

### Requirement: Results give an overview of what the plan finds

The Results step SHALL show: the number of jobs searched, read from the published catalogue snapshot; the number of jobs the plan matches; the exact number of distinct companies hiring for it; a world map marking the countries of the matched jobs, sized by count, with the top countries named; and a grid of the top hiring companies' logos. When the plan names countries, the map and the named countries SHALL show only those, because a posting listing several countries would otherwise read as a match outside the requested place. The chat SHALL remain available beside the overview at every viewport width at which the Results step is shown.

#### Scenario: The overview counts the plan's matches

- **WHEN** the visitor runs "Product designer roles in Brazil"
- **THEN** the overview shows the catalogue size, the matching job count, the distinct company count and the top companies' logos

#### Scenario: The map is limited to the requested country

- **WHEN** a plan filters on Brazil and some matched postings also list Mexico
- **THEN** the map and the named countries show Brazil only

#### Scenario: Jobs searched never counts on the request path

- **WHEN** the Results step renders
- **THEN** its jobs-searched figure comes from `GET /api/v1/stats/catalog`, not from a count computed for this request

### Requirement: Applying the plan writes it to the job list

**Show N jobs** SHALL apply the current plan through the job list's filter store in one write — the same query-string path a saved search uses — then close the modal and notify its host, so a host that stages its own edits (the filter modal) does not overwrite the applied filters.

#### Scenario: Show N jobs applies and closes

- **WHEN** the visitor chooses Show 799 jobs
- **THEN** the job list shows the plan's filters as ordinary removable chips, the modal closes, and the filter modal behind it closes too

### Requirement: Guests may use the AI search

The "Describe with AI" entry SHALL open the modal for signed-out visitors. `POST /api/v1/search/interpret` SHALL admit signed-out callers: their interpretation SHALL spend the service LLM credential, tagged with the search-intent feature, and SHALL NOT mint or resolve a per-user gateway credential. Signed-out callers SHALL be limited to 10 interpretations per hour per client IP; signed-in callers SHALL keep their own credential and their limit of 40 per hour per user. A signed-out visitor exceeding the limit SHALL be told to sign in to continue.

#### Scenario: A guest gets a plan

- **WHEN** a signed-out visitor submits a description
- **THEN** the interpretation runs on the service credential and the plan is shown

#### Scenario: A guest never gets a personal credential

- **WHEN** a signed-out caller is interpreted
- **THEN** no gateway credential is minted or looked up for any account

#### Scenario: The guest limit is per IP

- **WHEN** one IP has made 10 signed-out interpretations within the hour and makes another
- **THEN** the endpoint responds 429 and the modal invites the visitor to sign in

#### Scenario: Signed-in callers are unchanged

- **WHEN** a signed-in user submits a description
- **THEN** the interpretation spends that user's own credential under the 40-per-hour per-user limit

### Requirement: A CV can inform the first plan

On the Describe step a visitor, signed in or not, MAY attach a PDF CV. The CV SHALL be read through the deterministic resume extraction endpoint and SHALL NOT be stored. The derived seniority, categories and skills SHALL be shown on the attachment and appended to the first description sent to the interpreter; a CV with no description SHALL submit "Find jobs that fit my CV". Submitting by keyboard and by the send button SHALL behave identically.

#### Scenario: Attaching a CV shows what was read

- **WHEN** a visitor attaches a text-based PDF CV
- **THEN** the attachment shows the file name and the seniority and number of skills found

#### Scenario: A CV alone is enough to start

- **WHEN** a visitor attaches a CV, leaves the description empty and presses Enter
- **THEN** "Find jobs that fit my CV" is submitted with the CV's skills

#### Scenario: An unreadable CV is reported

- **WHEN** the attached PDF has no text layer
- **THEN** the attachment area shows the extraction error and nothing is submitted
