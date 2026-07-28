## ADDED Requirements

### Requirement: A browsing preset gives the assistant eyes on the current page

The assistant SHALL offer a `browse` preset alongside `chat` and `tailor`, for a
conversation held from a browser extension. As with every preset, it SHALL select
only the system prompt and the tool set: the discovery and tracking tools every
session gets, plus `read_current_page`. Its prompt SHALL instruct the agent to
read the page before guessing what the user is referring to, and to keep answers
short because the surface is a narrow column.

#### Scenario: A browsing session is offered the page tool

- **WHEN** a session running the `browse` preset builds its tool registry
- **THEN** the registry contains `read_current_page` alongside the discovery and tracking tools

#### Scenario: An unknown preset still runs

- **WHEN** a session records a preset the system does not recognise
- **THEN** it runs under the general chat prompt rather than failing, as it does today

### Requirement: A client asks for a browsing session when it creates one

Session creation SHALL accept the preset the client wants, admitting `chat` and
`browse` only. Omitting it SHALL create a chat, so an existing client keeps its
behaviour. `tailor` SHALL be refused here: a tailoring session is bound to a CV
and a vacancy, and one minted without that binding would register no CV tools and
be unusable.

#### Scenario: The extension creates a browsing session

- **WHEN** a session is created asking for the `browse` preset
- **THEN** the session is recorded as `browse` and its turns run with the browsing prompt and the page tool

#### Scenario: Asking for nothing still yields a chat

- **WHEN** a session is created with no preset named
- **THEN** it is recorded as `chat`

#### Scenario: A tailoring session cannot be minted here

- **WHEN** a session is created asking for the `tailor` preset
- **THEN** the request is refused and no session is created

### Requirement: The page tool reads through the caller's own browser channel

The assistant SHALL expose a `read_current_page` tool that returns what the
caller's browser is displaying: its url, title, headline and text. The tool SHALL
obtain it by attaching to the caller's browser-tool channel as an in-process
harness and issuing a `read_page` call, exactly as the agentic autofill attaches.
The channel SHALL be keyed by the user id the authenticating middleware resolved,
so the tool can never reach another user's browser.

#### Scenario: The page is returned as structured data

- **WHEN** the model calls `read_current_page` and the caller's extension answers with a snapshot
- **THEN** the tool result carries the page's url, title, headline and text as fields, not as prose

#### Scenario: The tool reads only the calling user's browser

- **WHEN** two users each have a browser attached and one of them runs a turn calling `read_current_page`
- **THEN** the call is served by that user's own browser and never by the other's

### Requirement: A missing browser is an error the model can act on

A `read_current_page` call made when no browser is attached SHALL return a tool
error naming the remedy — that the user should open the freehire side panel —
rather than ending the turn. The turn SHALL continue so the model can relay that
sentence to the user within the same answer.

#### Scenario: No extension is attached

- **WHEN** the model calls `read_current_page` and the caller has no browser attached to their channel
- **THEN** the tool returns an error result naming the side panel as the remedy
- **AND** the turn continues rather than ending as failed

### Requirement: The page tool is confined to the browsing preset

`read_current_page` SHALL NOT be registered for the `chat` or `tailor` presets. A
conversation held outside a browser extension has no page to read, and a tool
that can only fail spends the model's context without ever returning an answer.

#### Scenario: A chat session is not offered the page tool

- **WHEN** a session running the `chat` preset builds its tool registry
- **THEN** `read_current_page` is absent from it
