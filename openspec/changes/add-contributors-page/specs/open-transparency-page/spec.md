## ADDED Requirements

### Requirement: The contributor figure leads to the on-site contributors page

The open-source section's contributor count SHALL link to `/contributors` on this site
rather than to the repository's contributor graph on GitHub. The stars and forks figures
keep their existing links.

The moment a visitor is curious about who builds freehire is the moment the page should
keep them, not hand them to GitHub.

#### Scenario: The count is an on-site link

- **WHEN** a visitor inspects the contributor count in the open-source section
- **THEN** it links to `/contributors` on this site

#### Scenario: A missing count does not produce a broken link

- **WHEN** the GitHub leg degraded and no contributor count is available
- **THEN** the section renders its existing fallback and does not present a contributor figure linking anywhere
