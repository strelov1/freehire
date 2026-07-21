## Context

A Go naming review flagged four identifier deviations. All are local to a single
declaration plus its references; the only one that crosses a module boundary is
`FakeFreshness`, which is copied from `jobreality.Evidence` into the public
`jobview.Reality` wire struct.

## Goals / Non-Goals

**Goals:**
- Bring the four identifiers in line with the project's Go naming conventions.
- Keep the change purely mechanical — no behavior change, verified by the
  existing tests that already reference these fields.

**Non-Goals:**
- The wider set of borderline boolean fields (mostly external-API DTOs whose
  names mirror a foreign JSON schema) — left untouched deliberately.
- Any change to JSON wire tags or HTTP responses.

## Decisions

- **Rename the Go field, keep the JSON tag.** `jobview.Reality.FakeFreshness`
  serializes as `json:"fake_freshness"`. Renaming the Go field to
  `IsFakeFreshness` while leaving the tag string unchanged keeps the emitted
  wire contract byte-identical. Chosen over renaming the tag (which would be a
  breaking API change) and over leaving the field as-is (which would keep the
  deviation on a public type).
- **Rename in dependency order, compile between fields.** `jobreality` is the
  source of truth; `jobview` reads from it. Rename the `jobreality` field first,
  then update the `jobview` consumer, so `go build` stays meaningful at each
  step.

## Risks / Trade-offs

- [A stray reference is missed and the build breaks] → `grep` enumerated every
  reference up front (8 for `EvergreenText`, the `jobview` + `jobreality` sites
  for `FakeFreshness`, 4 for `Unread`); `go build ./...` + `go vet ./...` is the
  backstop.
- [Someone assumes the JSON key changed with the Go field] → the design note and
  the retained `json:"fake_freshness"` tag make the invariant explicit.
