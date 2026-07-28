## MODIFIED Requirements

### Requirement: Postings are identified by routing, not by markup

Installs customise the portal template freely — CSS classes, column order, and column count
differ between them — so the adapter SHALL identify postings by the portal's routing
invariants alone. A posting SHALL be recognised by any element carrying the route
`index.php?m=careers&p=showJob&ID=<n>`, whose captured `<n>` is the posting's `ExternalID`.
The route SHALL be read from an anchor's `href` and from an element's `onclick` navigation
target alike, because an install may make a whole table row clickable instead of linking its
title. The job title SHALL be the anchor's own text when the carrier is an anchor, and the
row's first cell when it is not, since a clickable row has no anchor text to read. The
adapter SHALL NOT depend on CSS classes, on the position of a listing column, or on the
number of listing columns.

#### Scenario: A rewritten template is parsed identically

- **WHEN** two boards serve listings with different markup, column counts, and CSS classes,
  but both link postings as `index.php?m=careers&p=showJob&ID=<n>`
- **THEN** both yield the same set of postings, with ids and titles taken from those links

#### Scenario: Duplicate links collapse to one posting

- **WHEN** a listing links the same posting id more than once (for example a title link and a
  separate "apply" link)
- **THEN** the adapter returns that posting once

#### Scenario: A clickable row is a posting

- **WHEN** a listing carries the posting route on a row's `onclick` handler and the listing
  contains no anchor carrying that route
- **THEN** the postings are still returned, each with the id from the route and the title from
  the row's first cell
