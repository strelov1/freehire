## MODIFIED Requirements

### Requirement: Verdict endpoint authentication and ownership

The verdict endpoint SHALL require an authenticated session (cookie-only) and
SHALL operate only on the caller's own single profile. It SHALL be a single
read-only `GET /me/profile/verdict` with no profile id in the path. When the
caller has no profile, the endpoint SHALL respond 404.

#### Scenario: Owner reads their verdict
- **WHEN** a signed-in user who has a profile requests `GET /me/profile/verdict`
- **THEN** the response is 200 with the verdict for their profile

#### Scenario: No profile yet
- **WHEN** a signed-in user who has not saved a profile requests `GET /me/profile/verdict`
- **THEN** the response is 404

#### Scenario: Unauthenticated request refused
- **WHEN** a request without a valid session cookie hits `GET /me/profile/verdict`
- **THEN** the response is 401
