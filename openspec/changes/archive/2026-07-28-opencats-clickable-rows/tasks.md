## 1. Recognise clickable rows

- [x] 1.1 Add a failing test for a listing whose postings ride an `onclick` handler with no
      anchor carrying the route (the `rms.adgonline.ca` shape).
- [x] 1.2 Generalise posting recognition from "anchor" to "route carrier": read the route from
      an anchor's `href` or an element's `onclick` navigation target.
- [x] 1.3 Take the title from the row's first cell when the carrier is not an anchor.
- [x] 1.4 Confirm anchor-carried listings still parse unchanged (existing tests stay green).

## 2. Board

- [x] 2.1 Add `rms.adgonline.ca/careers` to `sources/opencats.yml` as Vancouver Police
      Department.
- [x] 2.2 Verify the live board through the adapter: postings, titles, locations, descriptions.

## 3. Verification

- [x] 3.1 `go build ./... && go vet ./... && go test ./...` green.
- [x] 3.2 Re-crawl every opencats board and confirm no regression on the other nine.
