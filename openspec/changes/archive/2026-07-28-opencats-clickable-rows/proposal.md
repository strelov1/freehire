## Why

The OpenCATS adapter recognises a posting by its anchor, but an install in the wild carries
the same route on a clickable table row instead: `rms.adgonline.ca` (Vancouver Police
Department) navigates from `onclick="window.location.href='…p=showJob&ID=11'"` and has no
anchor at all. Its 5 postings were invisible — the listing has zero `<a>` elements carrying
the route.

This is the routing invariant holding while the carrier changes, which is exactly the case
the adapter claims to handle, so the requirement needs to say carrier rather than anchor.

## What Changes

- Recognise a posting whose route rides an element's `onclick` handler, not only an `<a href>`.
- Take the title from the row's first cell when there is no anchor text to read.
- Add the board to `sources/opencats.yml` (5 postings, live-verified).

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `opencats-source`: posting recognition is defined in terms of an anchor and its text; it
  must be defined in terms of whatever element carries the route, since an install may carry
  it on a row handler and then no anchor text exists.

## Impact

- `internal/sources/opencats.go` (+ tests), `sources/opencats.yml`.
- No schema change, no migration. Existing anchor-carried listings parse unchanged.
