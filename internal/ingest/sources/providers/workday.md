**Workday traps** (`workday`):

- **Empty board** (`emptyBoardListing`): `{"total":0,"jobPostings":[],"facets":[]}`; an unknown tenant answers 422, an unknown site 404, and an absent or null collection or a positive total over an empty FIRST page fails. Later pages are not held to that: `pageAll` stops successfully on an empty page even short of the latched total, so a mid-walk empty answer yields a partial listing.
