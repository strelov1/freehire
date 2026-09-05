## ADDED Requirements

### Requirement: The account area shows what is left to set up

The account area SHALL show a card listing the setup steps an account has not yet
completed, with a link to each, and SHALL stop showing it once every step is done.

The card SHALL be rendered at the top of the tracking section — the page a bare `/my`
already redirects to — rather than on an account landing page created to host it.

The steps SHALL be: a CV uploaded, a specialization and seniority, skills, location and
work mode, and one saved search with an alert.

The first four are the onboarding wizard's own steps, so the card measures the same thing
the funnel asked for and cannot drift from it. The fifth is what makes the product act
without a visit; a completeness meter that ends at a filled-in form measures paperwork
rather than activation.

The calculation SHALL read only state the client already holds, and SHALL NOT require a new
endpoint.

#### Scenario: A new account sees every step open

- **WHEN** a user who has completed no setup opens the tracking section
- **THEN** the card lists all five steps as outstanding, each linking to where it is done

#### Scenario: The card disappears when complete

- **WHEN** the last outstanding step is completed
- **THEN** the card is no longer rendered

#### Scenario: Completed steps are not re-asked

- **WHEN** a user has uploaded a CV but set no skills
- **THEN** the CV step reads as done and the skills step reads as outstanding

### Requirement: A quiet dot marks incomplete setup from anywhere

While any setup step is outstanding, the header menu button SHALL carry a dot.

The menu button, not the profile icon beside it: that icon is hidden below the `sm`
breakpoint, so on a phone it is not there to carry anything, and the menu button is the
one account control present at every width.

The dot SHALL NOT carry a count. The notification bell sits in the same corner and wins on
urgency; a second counted badge beside it would make two signals compete for one glance.

Its screen-reader equivalent SHALL be part of the button's own accessible name. An
aria-label replaces an element's contents as its name, so a visually-hidden span inside
the button would never be read.

#### Scenario: The dot marks an incomplete account

- **WHEN** a signed-in user with outstanding steps is on any page
- **THEN** the header account control shows a dot with no number

#### Scenario: The dot clears on completion

- **WHEN** the account's last outstanding step is completed
- **THEN** the dot is no longer shown
