## ADDED Requirements

### Requirement: Recognize a Talentglade employer board

The system SHALL recognize Talentglade career and posting URLs by their tenant subdomain.

#### Scenario: A posting identifies a tenant

- **WHEN** a submitted URL is a Talentglade application link on the employer's tenant subdomain
- **THEN** the system resolves that tenant as a Talentglade board

### Requirement: Import live postings by public ID

The adapter SHALL identify postings by public ID and verify the detail belongs to the requested posting.

#### Scenario: An expired posting shows an active replacement

- **WHEN** the detail endpoint marks the requested posting closed and displays a different active posting
- **THEN** the adapter does not import the replacement under the expired posting's ID
