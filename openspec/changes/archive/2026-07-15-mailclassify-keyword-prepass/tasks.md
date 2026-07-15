## 1. KeywordStatus (pure logic)

- [x] 1.1 RED: unit-test `KeywordStatus(subject, body)` — strong rejection wins
      over an ack opener; clear ack template → acknowledgement; explicit
      interview/offer/assessment → their signals; ambiguous opener alone → defer;
      unknown → defer.
- [x] 1.2 GREEN: implement precision-first ordered rules in
      `internal/mailclassify/keyword.go`. REFACTOR + simplify.

## 2. Classify fast-path

- [x] 2.1 RED: test that Classify with no candidates + a confident keyword email
      returns the keyword status and does NOT invoke the LLM (fake gen that fails
      if called); and that with candidates present, or an ambiguous email, the
      LLM IS called.
- [x] 2.2 GREEN: add the `len(Candidates)==0` keyword fast-path to Classify.

## 3. Verify

- [x] 3.1 `go build ./... && go vet ./... && go test ./internal/mailclassify/...
      ./internal/maillink/...` green.
