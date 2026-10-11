## ADDED Requirements

### Requirement: A failed résumé extraction is shown on the bank view with a retry action

The experience bank view SHALL read the résumé status (`GET /me/resume`'s `parse_status`/
`parse_detail`) and, when `parse_status` is `failed`, SHALL show a banner explaining that the
résumé could not be fully read and offering a retry action. The banner MUST NOT be shown when
`parse_status` is `ok` or absent (no résumé uploaded at all).

Activating the retry action SHALL call the retry endpoint and, on success, refresh the bank so
any newly-derived employments/achievements appear without a page reload. While waiting for the
outcome the view SHALL poll rather than claim an outcome it has not confirmed; if the wait is
abandoned before a terminal outcome is read, the view MUST NOT show the failed-extraction banner
on the strength of that alone — giving up asking is not the same as a confirmed failure.

#### Scenario: A failed extraction shows the banner

- **WHEN** the candidate opens the bank view and their résumé's `parse_status` is `failed`
- **THEN** a banner explains the résumé could not be fully read and offers a retry action

#### Scenario: A successful extraction shows no banner

- **WHEN** the candidate opens the bank view and their résumé's `parse_status` is `ok`
- **THEN** no résumé-extraction banner is shown

#### Scenario: No résumé uploaded shows no banner

- **WHEN** the candidate has never uploaded a résumé
- **THEN** no résumé-extraction banner is shown

#### Scenario: Retrying refreshes the bank on success

- **WHEN** the candidate activates the retry action and the retry succeeds
- **THEN** the bank reloads and shows whatever the retried extraction derived, without a page
  reload

#### Scenario: Abandoning the wait does not claim failure

- **WHEN** the candidate activates the retry action and the extraction is still pending when
  the view stops polling for it
- **THEN** the view shows no banner at all — not the failed-extraction banner — since nothing
  confirmed a failure
