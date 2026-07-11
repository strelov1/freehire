## ADDED Requirements

### Requirement: The main jobs feed offers a CV-similarity sort mode

The standalone jobs feed SHALL offer a sort control with two modes — "Newest" (the default, newest-added first) and "Recommended" (ranked by similarity to the caller's CV) — and in CV mode SHALL rank the feed via the recommendations endpoint while keeping every facet filter in effect. The selected sort SHALL round-trip through the URL (`sort=cv`) and the standalone list's persisted filter storage, so a reload, a shared link, or a return visit restores it. The `sort=cv` value SHALL be a frontend routing signal only and SHALL NOT be sent to the keyword search endpoint. The sort control SHALL appear only on the standalone feed, not on a company-scoped embedded feed. Free-text query is not combined with CV ranking; in CV mode the free-text query does not influence ranking while facet filters still apply.

The sort control SHALL be offered only to a signed-in user (the "Recommended" mode needs a CV, so a signed-out visitor has no use for it). When a user who cannot be ranked is nonetheless in CV mode, the feed SHALL prompt the appropriate next step instead of erroring: a signed-out user who reaches CV mode via a shared `sort=cv` link SHALL be prompted to sign in, and a signed-in user with no usable CV vector SHALL be prompted to add or update their CV. Because an empty CV feed is ambiguous at the API (no CV and no-match both return an empty list), the feed SHALL distinguish the no-CV prompt from an ordinary "no matches" state by whether a facet filter is applied.

#### Scenario: Switching to CV mode ranks the feed by the CV vector

- **WHEN** a signed-in user with a usable CV vector selects the "Recommended" sort on the feed
- **THEN** the feed re-fetches from the recommendations endpoint and lists open jobs ranked by similarity to the CV, and the URL carries `sort=cv`

#### Scenario: Facet filters still narrow the CV-sorted feed

- **WHEN** a user in CV mode has one or more facet filters applied (e.g. work mode, seniority)
- **THEN** the feed lists only jobs matching those facets, ranked by CV similarity

#### Scenario: CV sort round-trips on reload

- **WHEN** the feed is loaded with `sort=cv` in the URL for a signed-in user with a usable CV vector
- **THEN** the feed starts in CV mode ranked by the CV vector rather than the newest-first default

#### Scenario: The routing signal is not sent to the search endpoint

- **WHEN** the feed is in CV mode
- **THEN** the outgoing request goes to the recommendations endpoint and carries no `sort=cv` parameter to the keyword search endpoint

#### Scenario: Default (Newest) sort browses via keyword search

- **WHEN** a user selects "Newest" (or has not chosen a sort)
- **THEN** the feed lists open jobs newest-added first via the keyword search endpoint, and the URL carries no `sort` parameter

#### Scenario: The sort control is hidden for a signed-out user

- **WHEN** a signed-out user views the standalone feed
- **THEN** no sort control is shown (the default newest feed is served)

#### Scenario: Signed-out user reaching CV mode is prompted to sign in

- **WHEN** a signed-out user opens the feed with the CV sort active (e.g. a shared `sort=cv` link)
- **THEN** the feed shows a sign-in prompt and does not call the authenticated recommendations endpoint

#### Scenario: Signed-in user without a CV is prompted to upload one

- **WHEN** a signed-in user with no usable CV vector is in CV mode with no facet filter applied
- **THEN** the feed shows a prompt to add or update their CV rather than an error or a bare empty list

#### Scenario: CV mode with a non-matching filter shows a no-matches state

- **WHEN** a user in CV mode has a usable CV vector but the applied facet filters match no open job
- **THEN** the feed shows a non-error "no matches" state distinct from the upload prompt

#### Scenario: CV sort is absent on a company-scoped feed

- **WHEN** the feed is rendered embedded and scoped to a single company
- **THEN** no CV sort control is offered

## REMOVED Requirements

### Requirement: The recommendations page presents the feed with empty states

**Reason**: The standalone `/my/recommendations` page is removed; CV-ranked recommendations are now presented as a sort mode on the main jobs feed (see the added feed sort requirement), so the page, its view component, and its nav entry are deleted.

**Migration**: Use the main jobs feed (`/`) and select the "Recommended" sort, or link to `/?sort=cv`. The `GET /api/v1/me/recommendations` endpoint is unchanged and still backs the CV-ranked feed.
