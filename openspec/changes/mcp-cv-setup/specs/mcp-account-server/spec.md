## ADDED Requirements

### Requirement: A first CV can be added as text over MCP
The signed-in MCP server SHALL offer an OAuth grant an `add_cv` tool that stores the person's
own CV, passed verbatim as text, through the same use case the website's CV upload uses, and
SHALL return the profile values the CV suggests. The tool SHALL NOT invite an agent to compose,
summarise or embellish a CV; a person with no CV is sent to the website. As with a website
upload, skills found in the work history SHALL be added to the profile's skills once the
background read finishes, removable afterwards with `update_profile`.

#### Scenario: An account with no CV adds one
- **WHEN** an account with no stored CV calls `add_cv` with its CV text
- **THEN** the text is stored as the account's CV, its background extraction starts, and the
  result carries suggested specializations, seniorities and skills

#### Scenario: An account that already has a CV
- **WHEN** an account with a stored CV calls `add_cv`
- **THEN** the call is refused, the stored CV is unchanged, and the message names the website
  page where a CV is replaced

#### Scenario: Two first CVs at once
- **WHEN** two `add_cv` calls for an account with no CV run concurrently
- **THEN** exactly one CV is stored and the other call is refused as already having a CV

#### Scenario: Text outside the bounds
- **WHEN** `add_cv` is called with fewer than 100 or more than 30,000 characters
- **THEN** the call is refused and nothing is stored

#### Scenario: An API key cannot add a CV
- **WHEN** the tool list is read by a full-scope API key
- **THEN** `add_cv` is not among the tools

### Requirement: The account can see what setup is missing
The signed-in MCP server SHALL offer an OAuth grant a read-only `get_setup_status` tool that
reports, from existing state, the CV, job-search preferences, alerts, paid plan and
agents-may-apply switch, and exactly one next step naming a tool or a page.

#### Scenario: A new account
- **WHEN** an account with no CV calls `get_setup_status`
- **THEN** `next_step` names `add_cv`

#### Scenario: A CV but no preferences
- **WHEN** an account with a CV and no saved specializations calls `get_setup_status`
- **THEN** `next_step` names `update_profile`

#### Scenario: Core setup done, no paid plan
- **WHEN** an account with a CV, preferences and an active alert but no paid plan calls
  `get_setup_status`
- **THEN** `next_step` names `search_jobs` and `paid_plan` is listed as an optional step not done
