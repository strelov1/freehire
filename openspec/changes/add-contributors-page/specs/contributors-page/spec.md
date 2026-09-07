## ADDED Requirements

### Requirement: Public contributors showcase

The system SHALL serve a public, unauthenticated, server-rendered `/contributors` page
listing every person who has contributed to the freehire repository, each with their
avatar, their GitHub login, a summary of what they contributed, and a link to their own
profile page on this site.

The page SHALL be reachable from the site chrome and SHALL NOT require a client-side
fetch for its content to be present in the initial HTML.

#### Scenario: Page is public and server-rendered

- **WHEN** an anonymous visitor opens `/contributors`
- **THEN** the page responds 200 and every contributor is present in the server-rendered HTML

#### Scenario: Each entry links to its own page

- **WHEN** the showcase renders a contributor
- **THEN** that entry links to `/contributors/<login>` on this site, not to GitHub

### Requirement: Who counts as a contributor

A person SHALL appear on the showcase when they have at least one merged pull request or
at least one opened issue in the repository. Automated accounts SHALL NOT appear:
an account GitHub reports with account type `Bot`, or whose login ends in `[bot]`, is
excluded from every surface this capability serves — the showcase, the profile pages, and
the counts shown on them.

#### Scenario: A code contributor appears

- **WHEN** a person has one or more merged pull requests
- **THEN** they appear on the showcase

#### Scenario: An issue reporter appears

- **WHEN** a person has opened one or more issues and has never merged a pull request
- **THEN** they appear on the showcase, and their profile page renders with an empty pull-request list rather than failing

#### Scenario: Bots never appear

- **WHEN** the repository's contribution history includes `dependabot[bot]` or `github-actions[bot]`
- **THEN** neither appears on any page, and neither is counted in any figure the pages display

### Requirement: Maintainers are shown apart from contributors

The showcase SHALL present the repository's maintainers in their own group, visually and
structurally separate from the contributor group. Maintainer status SHALL be read from the
repository rather than from a list written by hand, so a new maintainer needs no code change
to appear correctly.

#### Scenario: The maintainer does not sit in the contributor grid

- **WHEN** the showcase renders and one account holds the overwhelming majority of commits as a maintainer
- **THEN** that account is rendered in the maintainer group and is absent from the contributor group

#### Scenario: Maintainers are not hardcoded

- **WHEN** repository maintainership changes
- **THEN** the next snapshot reflects it with no change to page code

### Requirement: Contributors are ordered by recency, not by volume

The contributor group SHALL be ordered by each person's most recent contribution, newest
first. It SHALL NOT be ordered by commit count, pull-request count, or any other measure of
volume.

#### Scenario: A newcomer is visible

- **WHEN** a person merges their first pull request today and every other contributor last contributed months ago
- **THEN** that person appears first in the contributor group

#### Scenario: Volume does not promote

- **WHEN** one contributor has eight merged pull requests from a year ago and another has one from last week
- **THEN** the more recent contributor is ordered first

### Requirement: Per-contributor profile page

The system SHALL serve a public, server-rendered `/contributors/<login>` page for each
person on the showcase, showing their avatar, their GitHub login and profile link, the date
of their first contribution, their merged pull-request count, their opened-issue count, and
the list of their merged pull requests with each title, its number, its merge date, and a
link to it on GitHub.

The page SHALL offer share actions for X and LinkedIn that share the page's own URL.

#### Scenario: A contributor's page renders their work

- **WHEN** a visitor opens `/contributors/<login>` for a person with merged pull requests
- **THEN** the page responds 200 and lists those pull requests by title with their merge dates

#### Scenario: An unknown login is not found

- **WHEN** a visitor opens `/contributors/<login>` for a login absent from the snapshot
- **THEN** the page responds 404

#### Scenario: Sharing carries the profile URL

- **WHEN** a visitor uses a share action on a profile page
- **THEN** the shared link is that contributor's profile URL on this site

### Requirement: Per-contributor social preview card

The system SHALL serve a 1200x630 PNG Open Graph card at `/contributors/<login>/og.png`
carrying that contributor's avatar, login, and headline figures, and each profile page
SHALL reference its own card in its Open Graph and Twitter metadata.

A card SHALL render even when the contributor's avatar cannot be fetched, degrading to the
site's existing monogram fallback rather than failing the response.

#### Scenario: A shared profile link previews the person

- **WHEN** a profile page URL is unfurled by a social platform
- **THEN** the preview image is that contributor's own card, not the site-wide card

#### Scenario: An unfetchable avatar does not fail the card

- **WHEN** the avatar image cannot be fetched while rendering a card
- **THEN** the card still renders with a monogram in the avatar's place and responds 200

#### Scenario: An unknown login has no card

- **WHEN** `/contributors/<login>/og.png` is requested for a login absent from the snapshot
- **THEN** the response is 404 and no image is returned

### Requirement: Pages read a committed snapshot, never the GitHub API

Neither the showcase, the profile pages, nor the card endpoint SHALL call the GitHub API
while serving a request. All contributor data SHALL come from a snapshot file committed to
the repository and read at build or request time from local state.

This keeps the pages outside GitHub's unauthenticated per-IP request budget, which the
repository already spends on the star badge and the `/open` page.

#### Scenario: Serving costs no GitHub request

- **WHEN** any contributors page or card is served
- **THEN** no request is made to the GitHub API

#### Scenario: An unreachable GitHub does not affect the pages

- **WHEN** the GitHub API is unreachable
- **THEN** every contributors page continues to render the last committed snapshot

### Requirement: A scheduled job refreshes the snapshot

A scheduled GitHub Actions workflow SHALL rebuild the snapshot once a day using the
workflow's own repository token, and SHALL commit the result only when the collected data
differs from what is committed.

The workflow SHALL fail rather than commit a snapshot it could not fully collect, so a
partial collection never silently shrinks the published list.

#### Scenario: Unchanged data produces no commit

- **WHEN** the workflow runs and the collected data matches the committed snapshot
- **THEN** no commit is created and no deployment is triggered

#### Scenario: A failed collection does not overwrite

- **WHEN** the workflow cannot complete its collection
- **THEN** the run fails, the committed snapshot is left untouched, and the live pages keep serving it

#### Scenario: A new contributor reaches the site

- **WHEN** a person's first pull request is merged
- **THEN** the next scheduled run commits them into the snapshot and they appear on the showcase

### Requirement: The contributors pages are crawlable

The system SHALL list `/contributors` and every `/contributors/<login>` page in the page
sitemap, so each profile is indexable on its own.

#### Scenario: Profiles are in the sitemap

- **WHEN** the page sitemap is requested
- **THEN** it contains `/contributors` and one entry per contributor profile
