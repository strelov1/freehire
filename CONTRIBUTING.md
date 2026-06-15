# Contributing to freehire

This guide exists to save both sides time.

## Philosophy

freehire's core is a small, opinionated pipeline: fetch → normalize → dedup →
upsert → enrich → serve. Everything a source adapter produces flows through one
schema and one wire shape.

**The extension point is the source, not the core.** Adding a company is one
entry in `sources.yml`. Adding an ATS platform is one new adapter in
`internal/sources` plus one line in `sources.All`. Adding a Telegram channel is
one entry in `channels.yml`. If your feature fits that shape, it is welcome.

PRs that bloat the core — new abstractions, config knobs, or error handling that
no current feature needs — will likely be rejected. Build each feature
correctly and idiomatically, neither gold-plated nor a placeholder.

## The One Rule

**You must understand your code.** If you cannot explain what your change does
and how it interacts with the rest of the system, your PR will be closed.

Using AI to write code is fine. Submitting AI-generated slop you have not read
and understood is not. If you use an agent, run it from the repository root so
it picks up `AGENT.md` automatically, and make sure it follows the rules there.

## Contribution Gate

All issues and PRs from new contributors are **auto-closed by default.**

Maintainers review auto-closed issues and reopen worthwhile ones. Approval
happens through maintainer replies on your issue:

- `lgtmi` — your future **issues** will not be auto-closed.
- `lgtm` — your future **issues and PRs** will not be auto-closed.

`lgtmi` does not grant the right to open PRs. Only `lgtm` does. Do not open a PR
before you have been approved with `lgtm` — open an issue first.

Questions, ideas, and help requests are not gated — use
[GitHub Discussions](https://github.com/strelov1/freehire/discussions).

## Quality Bar for Issues

Open issues through one of the GitHub issue templates. Keep them short,
concrete, and worth reading.

- Keep it concise. If it does not fit on one screen, it is too long.
- Write in your own voice. Do not paste raw LLM output as the issue body.
- State the bug or request clearly, and explain why it matters.
- For a bug, include reproduction steps and the relevant logs.
- If you want to implement the change yourself, say so.

A real, well-written issue may be reopened and approved.

## Before Submitting a PR

Do not open a PR unless you have already been approved with `lgtm`.

Run the same checks CI runs and make sure they pass:

```bash
go build ./...
go vet ./...
gofmt -l .              # must print nothing
go test ./...           # unit tests
```

If you touched anything Docker-dependent (queries, schema, handlers), also run
the integration suite locally:

```bash
go test -tags=integration ./...   # needs Docker (testcontainers)
```

Regenerate committed artifacts when their source changed, and commit the result:

- Changed `internal/db/queries/*.sql` or `migrations/` → `make sqlc`.
- Changed the Go contract types → `make gen-contracts`.

For the frontend (`web/`):

```bash
cd web
npm run check          # svelte-check
npm run build
```

## Adding a Source

This is the most welcome kind of contribution. See `AGENT.md` for the source
adapter contract and the dedup/lifecycle conventions. In short:

- An ATS board adapter implements the `Source` interface in `internal/sources`
  and is registered in `sources.All`.
- A single outbound-link resolver implements `LinkSource` in
  `internal/linksource` and is registered in `linksource.All`.
- Always validate board slugs against the live provider before adding them.

## Questions?

Ask in [GitHub Discussions](https://github.com/strelov1/freehire/discussions).
