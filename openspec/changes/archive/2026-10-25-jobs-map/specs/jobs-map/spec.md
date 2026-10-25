## ADDED Requirements

### Requirement: The job list opens a map of the current search

The `/jobs` list toolbar SHALL show a **Map** button at its left edge. It SHALL open a dialog that draws the jobs matching the list's current filters on a world map: one bubble per country, its area proportional to the country's job count, with the busiest places listed beside the map. The map SHALL zoom to fit what it draws and SHALL show the whole inhabited world when the drawn places span most of it. The counts SHALL come from the facets endpoint for the same filters the list uses.

#### Scenario: Opening the map

- **WHEN** a visitor on `/jobs` chooses Map
- **THEN** a dialog shows a bubble for each country with matching jobs and the busiest countries listed

#### Scenario: The map zooms to a narrow search

- **WHEN** the filters select only Brazil
- **THEN** the map shows South America rather than the whole world

### Requirement: One selected country shows its cities

When exactly one country is selected, the map SHALL draw that country's cities that it can place — matching the cities facet's names to a static coordinate table by case- and accent-insensitive name within that country — and SHALL list only cities in that country. It SHALL state how many of the country's jobs it placed in a city. A city it cannot place SHALL be omitted, not drawn elsewhere.

#### Scenario: Brazil shows its cities

- **WHEN** the filters select Brazil and the cities facet holds São Paulo, Campinas and Mexico City
- **THEN** São Paulo and Campinas are drawn and listed, and Mexico City is not

### Requirement: Clicking the map narrows the list's own filters

A click on a place SHALL change the list's filters, so the list behind the dialog updates. A country click SHALL make that country the only selected country; a click on the selected country SHALL deselect it. A city click SHALL select only that city and clear the country filter, while the map keeps drawing that country's cities — the location filters combine with OR on the server, so a city kept beside its country would select the whole country. **World view** SHALL clear the country and city filters together. Places SHALL be operable from the keyboard (Enter and Space) and SHALL show their name and count on hover and on focus.

#### Scenario: A country click replaces the selection

- **WHEN** Brazil is selected and the visitor clicks Mexico
- **THEN** only Mexico is selected and the list shows Mexico's jobs

#### Scenario: A city click narrows to the city

- **WHEN** Brazil is selected and the visitor clicks Campinas
- **THEN** only Campinas is selected, the list shows Campinas's jobs, and the map still shows Brazil's cities

#### Scenario: World view clears cities too

- **WHEN** Brazil and São Paulo are selected and the visitor chooses World view
- **THEN** neither a country nor a city filter remains

### Requirement: Remote worldwide is a toggle, not a place

The dialog SHALL show a **Remote worldwide** button with the number of matching jobs open to anyone anywhere. Turning it on SHALL select that filter and clear the country and city filters; turning it off SHALL remove it.

#### Scenario: Worldwide replaces a place

- **WHEN** Brazil is selected and the visitor turns on Remote worldwide
- **THEN** the filters hold remote worldwide and no country or city

### Requirement: The dialog fits the screen and always offers Show N jobs

The dialog SHALL not scroll at a 1280×720 laptop viewport or a 390×844 phone viewport. **Show N jobs** SHALL be visible without scrolling and SHALL close the dialog onto the list. Its count SHALL be the map's own count for the current filters, never a count left over from before the last click. A newer response SHALL always win over an older one still in flight.

#### Scenario: Show N jobs after a click

- **WHEN** the visitor clicks Brazil
- **THEN** the button reads the number of Brazil's jobs once the map has updated, and choosing it closes the dialog onto that list

#### Scenario: A slow old response does not win

- **WHEN** the visitor clicks two countries in quick succession and the first response arrives last
- **THEN** the map shows the second country's counts
