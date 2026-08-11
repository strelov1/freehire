## 1. Phone detection

- [ ] 1.1 In `internal/atscheck/atscheck.go`, replace `phoneRE` with a candidate pattern that matches a digit group optionally repeated with single separators (space, dot, hyphen), optionally prefixed by `+` or `00`, and allowing one parenthesized group of any length.
- [ ] 1.2 Add an unexported `hasPhone(cv string) bool` that walks the candidates, strips all but digits, drops a leading `00` or country prefix, and accepts a normalized length of 10–15. Keep it pure and I/O-free like the rest of `Score`.
- [ ] 1.3 Comment the digit band with the reason it exists — 15 is E.164's ceiling, 10 is what clears a year range and a date — so the next reader does not "fix" it downward.

## 2. Split the contact line item

- [ ] 2.1 In `formatCategory`, replace the combined item with two: email at weight 2 (`"Email address present"` / `"Add an email address near the top"`, `StatusWarn`) and phone at weight 2 (`"Phone number present"` / `"Add a phone number near the top"`, `StatusWarn`).
- [ ] 2.2 Update the category-maxima doc comment above `keywordMax` from `Format (8+4+2+2+2+2=20)` to the new item weights. Confirm `Format` still computes to 20 and the report to 100.

## 3. Tests

- [ ] 3.1 Table test for `hasPhone` accepting: a bare 11-digit run, `(NN) NNNNN-NNNN`, `+`-prefixed international, `(NNN) NNN-NNNN`, `NNN-NNN-NNNN`, dot- and space-separated forms, and a `00`-prefixed country prefix.
- [ ] 3.2 Table test for `hasPhone` rejecting: a four-digit year, a year range (`2019 - 2024`), a numeric date, a percentage, a currency amount, and a digit run longer than 15.
- [ ] 3.3 Test that a CV with an email and no phone yields a passing email item and a warning phone item whose text does not mention email — the exact regression that prompted this change.
- [ ] 3.4 Test that a CV with both details scores the same Format total as before the split.

## 4. Verify

- [ ] 4.1 `go build ./... && go vet ./...`
- [ ] 4.2 `go test ./internal/atscheck/`
- [ ] 4.3 `go vet -tags=integration ./...`
- [ ] 4.4 Re-run the ATS report against the CV that reported the false warning and confirm the phone item passes.

## 5. Announce

- [ ] 5.1 Offer a `/blog` changelog entry — the report's advice was visibly wrong to anyone outside US phone conventions, so this is user-facing.
