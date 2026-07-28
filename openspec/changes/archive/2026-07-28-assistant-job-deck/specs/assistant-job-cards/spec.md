## ADDED Requirements

### Requirement: The agent presents vacancies through a typed tool

The agent SHALL present every vacancy it shows the user by calling a
`present_jobs` tool, and SHALL NOT write a job link into its prose. One call
carries an optional group `heading` and one to ten entries, each of which is a
vacancy's `public_slug` copied from a search or read result, a one-sentence
`note`, and optionally up to four `why_fits` phrases and up to three `concerns`.
The `note` SHALL NOT restate the vacancy's title, company, location or seniority,
because the card already shows them.

Presenting two groups (for example a shortlist and a wider set) SHALL be two
calls with different headings, not one call with prose between the entries.

#### Scenario: A recommendation is a tool call, not prose

- **WHEN** the agent has screened search results and is ready to recommend vacancies
- **THEN** it calls `present_jobs` with those vacancies' `public_slug` values and a rationale for each, and its own text carries no job link

#### Scenario: The rationale does not duplicate the card

- **WHEN** the agent writes a `note` for a presented vacancy
- **THEN** the note explains why the vacancy is worth the user's time without repeating the title, company, location or seniority the card renders

#### Scenario: Two groups are two calls

- **WHEN** the agent wants to separate strong matches from weaker ones
- **THEN** it makes one `present_jobs` call per group, each with its own heading

### Requirement: A presented slug is validated before the deck is shown

The `present_jobs` tool SHALL resolve every submitted slug against the catalogue
before the deck reaches the user. Slugs that resolve are reported as `presented`;
slugs that do not are dropped and reported as `dropped` with the offending slug
and a reason, so the model can correct itself within the same turn. A call in
which no slug resolves SHALL fail with an error naming the unresolved slugs.

The result SHALL be a receipt of slugs, not the vacancies' payload: the search
result that produced them is already in the model's history, and repeating it
would duplicate the most expensive part of the context.

#### Scenario: Every slug resolves

- **WHEN** the model calls `present_jobs` with slugs that all exist
- **THEN** the result lists them as `presented`, `dropped` is empty, and no vacancy payload is echoed back into the conversation

#### Scenario: Some slugs do not resolve

- **WHEN** the model calls `present_jobs` with a mix of real and unknown slugs
- **THEN** the real ones are `presented`, the unknown ones appear in `dropped` with their reason, and the model may present a replacement without regenerating the rationale for the entries that survived

#### Scenario: No slug resolves

- **WHEN** every slug in a `present_jobs` call is unknown
- **THEN** the call returns an error naming the unresolved slugs, and no deck is rendered

### Requirement: A presented set renders as one contiguous deck

The chat SHALL render one `present_jobs` call as a single deck: its heading, if
given, followed by its cards in the order the model listed them, with no prose
between the cards. Each card SHALL carry the model's `note`, its `why_fits` and
its `concerns` inside the card's own border, below the vacancy's data.

A `present_jobs` call SHALL NOT also appear in the transcript's tool-activity
list, which would put a redundant progress chip above the deck it produced.

#### Scenario: Cards are not split by prose

- **WHEN** a reply presents six vacancies in one call
- **THEN** the six cards render as one uninterrupted stack under a single heading

#### Scenario: The rationale sits inside the card

- **WHEN** a presented vacancy has a note, `why_fits` and `concerns`
- **THEN** all three render within that card's border, beneath the vacancy's own data, and not as separate paragraphs in the message

#### Scenario: The presenting call is not shown as tool activity

- **WHEN** a turn contains a `present_jobs` call alongside other tool calls
- **THEN** the other calls appear in the tool-activity list and `present_jobs` does not, because its deck is already on screen

### Requirement: A deck renders only after its call has succeeded

The chat SHALL render a deck only once the `present_jobs` call's result has
arrived and is not an error. A call still in flight SHALL render a placeholder,
and a failed call SHALL render nothing.

Rendering from the call's arguments alone would show a deck built from
unvalidated slugs and then replace it when the model corrected itself; waiting
for the result means the user never sees a deck that the backend rejected.

#### Scenario: A rejected call shows no deck

- **WHEN** a `present_jobs` call fails because none of its slugs resolve, and the model retries with corrected slugs
- **THEN** only the corrected deck is ever shown

## MODIFIED Requirements

### Requirement: Cards are hydrated from the structured job API

A job card SHALL fetch the posting's structured jobview by slug
(`api.getJob` → `GET /api/v1/jobs/:slug`) and render the shared `JobRow`
component from that data, so the vacancy's own facts are always accurate and
never model-authored — the model contributes only the rationale attached to the
card. While loading it SHALL show a placeholder; if the fetch fails it SHALL fall
back to a plain link, never breaking the message. Repeated slugs within a session
SHALL be served from a client cache.

#### Scenario: Successful hydration

- **WHEN** a card mounts for a slug the tool has validated
- **THEN** it shows a loading placeholder, fetches the jobview once, and renders the `JobRow` card with that data plus the model's rationale

#### Scenario: Unknown or failed slug degrades to a link

- **WHEN** the jobview fetch errors for a slug the tool accepted (for example the network is down)
- **THEN** that entry renders the job URL as a plain link and the rest of the deck is unaffected

#### Scenario: Cached on repeat

- **WHEN** the same slug is rendered again (same or later message in the session)
- **THEN** the cached jobview is reused with no additional network request

### Requirement: Cards are interactive and app-consistent

A job card SHALL be the same `JobRow` used elsewhere in freehire (its bookmark
and styling), and SHALL open the job detail in a **new tab** so the chat session
stays open. A deck SHALL be laid out to match the chat column, and its cards
SHALL be spaced as one group rather than as independent blocks, so a
recommendation reads as one surface.

#### Scenario: Opening a card keeps the chat

- **WHEN** the user clicks a job card in the chat
- **THEN** the job detail opens in a new browser tab and the assistant chat remains open and attached in the current tab

#### Scenario: A card in a deck is still bookmarkable

- **WHEN** the user saves a vacancy from a card inside a deck
- **THEN** it is saved exactly as it would be from the same card elsewhere in the app

## REMOVED Requirements

### Requirement: Job links in an assistant reply render as job cards

**Reason**: A vacancy is no longer shown by writing its link into prose, so there
is no link left to unfurl. Recovering cards by regex-matching the finished reply
made the model's rationale unreachable as data, let prose split a set of cards,
and turned an indented rationale into a code block.

**Migration**: The agent calls `present_jobs`; the chat renders the deck that call
describes. A job link that still appears in prose renders as an ordinary markdown
link. Assistant transcripts recorded before this change show links where they
previously showed cards, because their stored turns contain no `present_jobs`
call to replay.

### Requirement: The agent produces unfurlable job links

**Reason**: Replaced by the tool obligation. The prompt no longer asks for a
canonical URL per line, because a URL in prose is no longer how a vacancy is
shown.

**Migration**: See "The agent presents vacancies through a typed tool". The
`public_slug` is still carried by every job-search and job-read result, and is
still copied rather than constructed — it is now copied into a tool argument
instead of into a URL.
