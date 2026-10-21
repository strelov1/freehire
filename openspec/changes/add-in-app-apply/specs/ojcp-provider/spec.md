## MODIFIED Requirements

### Requirement: ApplyPath projection from captured forms

The system SHALL project a posting's stored application form into an OJCP `ApplyPath`
carrying `type`, `ats_provider`, `required_fields` and `supports_agent_submission`.

`supports_agent_submission` SHALL be true only for postings this deployment can actually
submit to unattended. A posting on an ATS whose form we can read but not submit — one
gated by a challenge we do not solve — SHALL report `false`.

A posting with no captured form SHALL report an `external_redirect` path rather than
omitting `apply_paths`.

An employer vacancy that takes applications on freehire SHALL report a single
`platform_native` path whose URL is the posting's page on this site, naming no ATS provider,
listing the required employer questions when it has any, with `supports_agent_submission`
false. The manifest SHALL declare `platform_native` among its apply path types.

#### Scenario: Posting with a captured, submittable form

- **WHEN** a posting has a stored apply form on a platform auto-apply supports
- **THEN** its `ApplyPath` names the ATS provider and lists the form's required fields
- **AND** `supports_agent_submission` is true

#### Scenario: Posting on a challenge-gated platform

- **WHEN** a posting's ATS gates submission behind a challenge this deployment does not solve
- **THEN** `supports_agent_submission` is false
- **AND** the required fields are still published, so an agent can still prepare a human hand-off

#### Scenario: Posting with no captured form

- **WHEN** a posting has no stored application form
- **THEN** its `apply_paths` carries a single `external_redirect` entry pointing at the source URL

#### Scenario: Employer vacancy taking applications on freehire

- **WHEN** a posting is an employer vacancy in `freehire` apply mode
- **THEN** its `apply_paths` carries a single `platform_native` entry pointing at the posting's page on this site
- **AND** it names no ATS provider, lists the required questions, and `supports_agent_submission` is false
