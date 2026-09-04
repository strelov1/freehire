# Contributing to freehire

Contributions are welcome. This guide exists to save both sides time.

## Philosophy

freehire's core is a small, opinionated pipeline: fetch → normalize → dedup →
upsert → enrich → serve. Everything a source adapter produces flows through one
schema and one wire shape.

**The extension point is the source, not the core.** Adding a company is one row
in the `boards` catalog — submit it through the "contribute a board" form on the
site, no PR needed. Adding an ATS platform is one new adapter in
`internal/ingest/sources` plus one line in `sources.All`. Adding a Telegram channel is
one row in `telegram_channels`. If your feature fits that shape, it is welcome.

Changes that bloat the core — new abstractions, config knobs, or error handling
that no current feature needs — are a harder sell. Build each feature correctly
and idiomatically, neither gold-plated nor a placeholder.

## The One Rule

**You must understand your code.** If you cannot explain what your change does
and how it interacts with the rest of the system, it will not be merged.

Using AI to write code is fine — this repository is built with it. Submitting
generated code you have not read is not. If you use an agent, run it from the
repository root so it picks up `AGENTS.md` automatically, and check that it
followed the rules there.

## Opening an issue

Use one of the [issue templates](.github/ISSUE_TEMPLATE). Bug reports, source
requests, and feature ideas are all fair game.

- Keep it concise. If it does not fit on one screen, it is probably two issues.
- Write in your own voice. A pasted LLM transcript is hard to act on.
- For a bug, include reproduction steps and the relevant logs.
- If you want to implement it yourself, say so — we will not race you.

Questions, half-formed ideas, and help requests belong in
[Discussions](https://github.com/strelov1/freehire/discussions) instead.

## Opening a pull request

Small PRs get reviewed fastest. For anything large or architectural, open an
issue first so we can agree on the shape before you spend the evening on it —
that is a suggestion, not a gate.

Run the same checks CI runs:

```bash
go build ./...
go vet ./...
gofmt -l .              # must print nothing
go test ./...           # unit tests
```

Note that `go test ./...` compiles no file behind the `integration` build tag.
If you changed a signature, `go vet -tags=integration ./...` is the cheap guard —
seconds, no Docker. If you touched anything Docker-dependent (queries, schema,
handlers), run the suite itself:

```bash
go test -tags=integration ./...   # needs Docker (testcontainers)
```

Regenerate committed artifacts when their source changed, and commit the result:

- Changed `internal/platform/db/queries/*.sql` or `migrations/` → `make sqlc`.
- Changed the Go contract types → `make gen-contracts`.

For the frontend (`web/`):

```bash
cd design-system && pnpm install && pnpm build   # design-system must build first
cd ../web && pnpm install && pnpm run check && pnpm run build
```

## Adding a source

This is the most welcome kind of contribution. See `AGENTS.md` for the source
adapter contract and the dedup/lifecycle conventions. In short:

- An ATS board adapter implements the `Source` interface in `internal/ingest/sources`
  and is registered in `sources.All`.
- A single outbound-link resolver implements `LinkSource` in
  `internal/ingest/linksource` and is registered in `linksource.All`.
- Always validate board slugs against the live provider before adding them — a
  board that yields nothing looks identical to a healthy one in the logs.

## Questions?

Ask in [GitHub Discussions](https://github.com/strelov1/freehire/discussions).
