## MODIFIED Requirements

### Requirement: Verdict endpoint authentication and ownership

The profile sub-resource endpoints SHALL require an authenticated session
(cookie-only) and SHALL operate only on the caller's own single profile. These
endpoints are the market-coverage verdict and the CV ATS-readiness report, and
SHALL be addressed without a profile id in the path: the verdict as
`GET /me/profile/verdict`, the ATS report as `GET`/`POST /me/profile/ats-report`.
When the caller has no profile, these endpoints SHALL respond 404.

#### Scenario: Owner reads their verdict
- **WHEN** a signed-in user who has a profile requests `GET /me/profile/verdict`
- **THEN** the response is 200 with the verdict for their profile

#### Scenario: Owner reads their ATS report
- **WHEN** a signed-in user who has a profile requests `GET /me/profile/ats-report`
- **THEN** the response is 200 with the report for their profile

#### Scenario: No profile yet
- **WHEN** a signed-in user who has not saved a profile requests `GET /me/profile/verdict` or `GET /me/profile/ats-report`
- **THEN** the response is 404

#### Scenario: Unauthenticated request refused
- **WHEN** a request without a valid session cookie hits `GET /me/profile/verdict` or `/me/profile/ats-report`
- **THEN** the response is 401
