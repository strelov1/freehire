## ADDED Requirements

### Requirement: A dedicated tailoring route bootstraps and seeds the session

The system SHALL serve a beta-gated route `/tailor/[slug]` that, on load, bootstraps the
tailored CV for the vacancy, starts an agent session seeded with the tailoring context, and
auto-starts the agent so the dialogue begins without the user typing a first message. Failures
of the bootstrap (no cached analysis, no résumé) MUST surface an actionable message rather than
a blank page.

#### Scenario: Opening the route starts the tailoring dialogue

- **WHEN** a beta user opens `/tailor/<slug>` for a vacancy with a cached fit analysis and a stored résumé
- **THEN** a tailored CV is created, an agent session is started seeded to reframe it, and the agent begins on its own (no empty chat awaiting a first message)

#### Scenario: A missing precondition is explained, not blank

- **WHEN** the bootstrap fails because there is no cached analysis or no résumé
- **THEN** the page shows an actionable message (run the analysis / add a résumé) instead of an empty surface

### Requirement: The tailoring surface has its own full-width layout

The system SHALL render `/tailor/[slug]` in its own full-width layout — without the /my account
navigation rail or the /jobs page chrome, and without max-width card framing — so the chat and
the artifact panel use the full viewport width.

#### Scenario: The surface is full-width

- **WHEN** the tailoring surface renders on a wide viewport
- **THEN** it spans the full width with no /my nav rail and no max-width container, chat on the left and the artifact panel on the right

### Requirement: The chat is a single reusable component

The system SHALL implement the agent chat (transport, session lifecycle, message list,
composer) as one reusable component used by BOTH the tailoring surface and `/my/assistant`, so
the chat behaviour is defined in a single place. The `/my/assistant` chat MUST remain
behaviourally unchanged after the extraction.

#### Scenario: Both surfaces share one chat implementation

- **WHEN** the chat is used on `/tailor/[slug]` and on `/my/assistant`
- **THEN** both render the same component, and `/my/assistant` behaves as it did before the extraction (send, queue, switch session, delete)

### Requirement: The artifact panel has CV, job-description, and verdict tabs

The system SHALL present a tabbed artifact panel beside the chat with three tabs — the tailored
CV rendered as its ATS PDF, the vacancy's job description, and the fit verdict (overall score,
recommendation, and the requirement split `missing_have` / `missing_gap`). The panel's width
SHALL be user-adjustable via a draggable splitter, clamped to a sensible range.

#### Scenario: Switching tabs shows each artifact

- **WHEN** the user selects the CV, Job description, or Verdict tab
- **THEN** the panel shows the live CV PDF, the vacancy text, or the fit verdict respectively

#### Scenario: The panel resizes and clamps

- **WHEN** the user drags the splitter beyond the allowed range
- **THEN** the panel width is clamped to the minimum/maximum rather than collapsing or overflowing

### Requirement: The CV tab reflects the agent's edits

The system SHALL refresh the CV artifact after each completed agent turn, so an edit the agent
just made through `freehire cv edit` is reflected without a manual reload.

#### Scenario: A completed edit turn updates the CV

- **WHEN** an agent turn completes (it may have edited the tailored CV)
- **THEN** the CV tab re-renders the current PDF

### Requirement: The fit page links to the tailoring surface

The "tailor my CV" entry point on the fit page SHALL navigate to `/tailor/[slug]` for that
vacancy.

#### Scenario: The CTA opens the tailoring surface

- **WHEN** a beta user with a cached analysis clicks "Tailor my CV" on `/jobs/<slug>/fit`
- **THEN** they land on `/tailor/<slug>`
