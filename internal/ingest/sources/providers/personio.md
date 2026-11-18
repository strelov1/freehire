# Personio

The default XML feed is a complete listing without pagination. Require its
native `workzag-jobs` root and valid, unique position IDs and names: another XML
root must never become an empty board. Direct children must be positions, with
only formatting whitespace at the root: native-root error envelopes and unknown
children fail the listing instead of silently becoming empty or partial boards. Verified on 2026-11-11: `ifunds` returns
HTTP 200 with an empty `<workzag-jobs>` document while its expired posting
2335254 returns HTTP 404. Its German `search.json` independently returns `[]`. An unknown tenant
`freehire-nonexistent-20261111` returns HTTP 404 on both XML and search endpoints.

Only an XML HTTP 404 selects the full `search.json?language=de` fallback.
That fallback requires a present JSON array with valid, unique IDs and names;
`null`, an object, or malformed entries fail the listing. An explicit empty
native XML document or JSON array may authorize the ordinary empty-board sweep.
English feeds and detail pages improve bodies only, never listing membership;
their failures must not drop a position from the default listing.
