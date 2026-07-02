## ADDED Requirements

### Requirement: Global footer component

The web app SHALL render a single global site footer on every route, implemented as
a dedicated `Footer.svelte` component and mounted once from the root layout (not
inlined per route). The footer SHALL stay pinned to the bottom of the viewport on
sparse pages via the existing column layout.

#### Scenario: Footer present on every page

- **WHEN** a user loads any route in the SPA
- **THEN** the site footer is rendered below the main content
- **AND** on a short page the footer sits at the bottom of the viewport rather than
  floating mid-screen

#### Scenario: Footer is a reusable component

- **WHEN** the root layout renders
- **THEN** it renders `<Footer />` rather than inline `<footer>` markup

### Requirement: Multi-column layout with grouped navigation

The footer SHALL present a restrained multi-column layout: a brand block (project
name + one-line tagline + social icons) and navigation columns that group existing
site routes. Link groups SHALL be:

- **Product**: Jobs, Companies, Collections, Recruiters
- **Resources**: CLI, API docs
- **Company**: For companies, Submit a job

Internal links SHALL use `$app/paths` `resolve()`. The layout SHALL remain clean and
uncluttered — no links or sections beyond those listed.

#### Scenario: Navigation groups link to real routes

- **WHEN** a user clicks a footer navigation link
- **THEN** they are taken to the corresponding in-app route (e.g. Jobs → `/jobs`,
  API docs → `/docs/api`)

### Requirement: Social links including LinkedIn

The footer SHALL expose social links for GitHub
(`https://github.com/strelov1/freehire`), LinkedIn
(`https://linkedin.com/company/freehire-dev/`), and Telegram
(`https://t.me/freehiredev`), each rendered with its `ProviderIcon` brand mark.
External links SHALL open in a new tab with `target="_blank"` and
`rel="noopener noreferrer"`.

#### Scenario: LinkedIn link present and safe

- **WHEN** a user views the footer
- **THEN** a LinkedIn link pointing at `https://linkedin.com/company/freehire-dev/`
  is shown with the LinkedIn icon
- **AND** it opens in a new tab with `rel="noopener noreferrer"`

#### Scenario: GitHub and Telegram links retained

- **WHEN** a user views the footer
- **THEN** the existing GitHub and Telegram links are still present, each with its
  brand icon and opening in a new tab

### Requirement: Bottom bar

The footer SHALL include a bottom bar containing a copyright line and an
open-source note, visually separated from the columns by a thin border.

#### Scenario: Copyright and open-source note shown

- **WHEN** a user views the footer
- **THEN** a bottom bar shows a copyright line and an open-source note

### Requirement: Responsive and theme-faithful

The footer SHALL be responsive — columns stack vertically on narrow viewports and
sit side by side on wide ones — and SHALL use only the existing flat-neutral design
tokens (oklch grayscale, thin borders, system fonts). It SHALL NOT introduce new
fonts or colors beyond the existing brand icon marks, and SHALL render correctly in
both light and dark themes.

#### Scenario: Columns stack on mobile

- **WHEN** the footer is viewed on a narrow (mobile) viewport
- **THEN** the brand block and navigation columns stack vertically without horizontal
  overflow

#### Scenario: Adapts to theme

- **WHEN** the app is in dark mode
- **THEN** the footer uses the dark-theme tokens (foreground/muted/border) and remains
  legible
