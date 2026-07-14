# assistant-job-cards Specification

## Purpose
TBD - created by archiving change assistant-job-cards. Update Purpose after archive.
## Requirements
### Requirement: Job links in an assistant reply render as job cards

An assistant message SHALL render every freehire job reference
(`https://freehire.dev/jobs/<slug>`, protocol/host-optional bare `/jobs/<slug>`)
as a job card showing the posting's real data, rather than a plain link. The
surrounding prose SHALL still render as markdown, in order.

#### Scenario: A reply listing jobs shows cards

- **WHEN** the agent replies with several `https://freehire.dev/jobs/<slug>` links (e.g. a recommended shortlist)
- **THEN** each link is replaced, in place, by a job card with the posting's logo, title, tags, skills, and posted time, and any text between links renders as normal markdown

#### Scenario: A non-job link is left alone

- **WHEN** the reply contains a link that is not a freehire job link (e.g. a company page or an external URL)
- **THEN** it renders as an ordinary markdown link, not a card

#### Scenario: Duplicate and punctuation-adjacent links

- **WHEN** the same job link appears twice, or a link is immediately followed by punctuation (`…/jobs/foo.`)
- **THEN** each occurrence renders a card for the correct slug (trailing punctuation excluded), and the two cards for the same slug do not trigger a second fetch

### Requirement: Cards are hydrated from the structured job API

A job card SHALL fetch the posting's structured jobview by slug
(`api.getJob` → `GET /api/v1/jobs/:slug`) and render the shared `JobRow`
component from that data, so the card is always accurate (never model-authored).
While loading it SHALL show a placeholder; on a fetch error or unknown slug it
SHALL fall back to a plain link, never breaking the message. Repeated slugs
within a session SHALL be served from a client cache.

#### Scenario: Successful hydration

- **WHEN** a card mounts for a valid slug
- **THEN** it shows a loading placeholder, fetches the jobview once, and renders the `JobRow` card with that data

#### Scenario: Unknown or failed slug degrades to a link

- **WHEN** the jobview fetch 404s or errors
- **THEN** the card renders the original job URL as a plain link and the rest of the message is unaffected

#### Scenario: Cached on repeat

- **WHEN** the same slug is rendered again (same or later message in the session)
- **THEN** the cached jobview is reused with no additional network request

### Requirement: Cards are interactive and app-consistent

A job card SHALL be the same `JobRow` used elsewhere in freehire (its bookmark
and styling), and SHALL open the job detail in a **new tab** so the chat session
stays open. Cards SHALL be laid out to match the chat column (bounded width,
spacing) so the reply reads as one coherent surface.

#### Scenario: Opening a card keeps the chat

- **WHEN** the user clicks a job card in the chat
- **THEN** the job detail opens in a new browser tab and the assistant chat remains open and attached in the current tab

### Requirement: The agent produces unfurlable job links

The `using-freehire` skill SHALL direct the agent to present each recommended or
listed job as its canonical `https://freehire.dev/jobs/<public_slug>` URL, so the
chat can unfurl it into a card. The `public_slug` comes from the CLI's job
payload; the skill SHALL construct (or copy) the canonical URL from it.

#### Scenario: Recommending jobs yields cards

- **WHEN** the agent uses the freehire CLI to find jobs and then recommends them to the user
- **THEN** each recommendation includes the job's `https://freehire.dev/jobs/<public_slug>` URL, which the chat renders as a card

