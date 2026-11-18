# A region facet at state or province level (GeoNames admin levels)

This project does not offer a filter by state, Land or province. Places resolve offline to a city
(`cities`) and a metro area (`metros`); countries and regions sit above them.

## Why this is out of scope

- Local spellings already resolve: the location dictionary is built from GeoNames `cities1000`, and
  every place of 15,000 people or more keeps its alternate names (smaller ones keep only their own
  name), so "München", "Muenchen" and "Monaco di Baviera" are one place at ingest,
  and the `cities` filter resolves them too (#3651). "Near a place" is the `metros` facet.
- The missing piece is the admin level, and demand for it was measured: searches naming a state or
  Land were about 0.13% of all searches (2026-10-27), almost none for German Länder. That does not
  pay for a new facet plus a backfill of the catalogue. Which level would serve also differs by
  country, as the reporter's GeoNames tally in #3653 showed (ADM1 for Albania, ADM2 for Belgium,
  a special case for Slovenia).
- A live GeoNames API is out either way: the dictionaries are offline by design, and the free
  quota is far below ingest volume.

Revisit if searches naming a state or province grow well past that share.

## Prior requests

- #3653: "City names and geographical regions"
