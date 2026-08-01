## Why

`internal/pgconv` states its own purpose: these conversions live in one place "instead of being
re-declared in every package that touches the database". Sixteen packages import it.
`internal/handler` — the largest in the repo — carried private twins under different names, so a
reader grepping for the conversion found several spellings and could not tell whether they agree.

Body-identical, verified:

| handler | pgconv |
|---|---|
| `tsFromPtr(*time.Time) pgtype.Timestamptz` | `Timestamptz` |
| `int4Ptr(pgtype.Int4) *int` | `IntPtr` |

And a **third the finding does not name**: `api_keys.go` wrote `pgconv.Timestamptz` out inline as
a `var` plus an `if`, four lines doing one function's job.

Small in isolation. It is the same habit that produced `pgText` in `companies.go` (deleted in
#1409, where it turned out to duplicate `pgconv.Text`), and it is what keeps the `pgtype`
vocabulary alive inside the transport layer.

## What Changes

- `tsFromPtr` and `int4Ptr` are deleted; their two call sites use `pgconv.Timestamptz` and
  `pgconv.IntPtr`.
- `api_keys.go`'s inline twin collapses to one call.
- `hardconstraint_inputs.go` stops importing `pgtype` altogether.
- No behaviour change: the deleted bodies are character-for-character the shared ones.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

(none) — no requirement-level behaviour change. `tasks.md` is the real artifact; the change
archives with `--skip-specs`.

## Impact

- `internal/handler/match_analysis.go`, `hardconstraint_inputs.go`, `api_keys.go`.
- Deliberately left: six `pgtype` literals the handler still builds. Four are `pgtype.Timestamp`
  (pgconv has `Timestamptz`, a different type), one is `pgtype.UUID`, and one takes a non-pointer
  so there is no nil↔NULL question to answer. They are plain literals, not converters; inventing
  `pgconv` functions for them would be infrastructure ahead of need.
