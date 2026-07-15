## 1. Vocabulary + keyword

- [x] 1.1 RED: test that `Sanitize` keeps `incomplete_application` (not coerced to
      other) and that it is NOT in the stage-advancing map; test `KeywordStatus`
      returns it for templated "complete your application" phrasing, ordered
      before the acknowledgement opener.
- [x] 1.2 GREEN: add `SignalIncompleteApplication` to the vocab + `validSignals`,
      a keyword rule before acknowledgement, and the prompt category.

## 2. Frontend badge

- [x] 2.1 Add the `incomplete_application` label + colour to `emailStatus.ts`;
      `svelte-check` clean.

## 3. Verify

- [x] 3.1 `go build/vet/test ./internal/mailclassify/... ./internal/maillink/...`
      green; frontend build passes.
