## 1. Settle the contract before writing either client

- [ ] 1.1 Write the tool list down in one place both repos can read: names, arguments, and every outcome each primitive can return, taken from `freehire-extension`'s `lib/combobox.ts` and `lib/tools/executor.ts` as they stand. This is the artefact the two implementations are checked against, so it precedes both.
- [ ] 1.2 Decide the MCP question left open in design: do the browser primitives appear unconditionally, or behind an opt-in? An MCP host gaining browser-*writing* tools by default is a larger grant than its current read-and-apply tools.
- [x] 1.3 Confirm on a running hire that an API key authenticates `role=harness` and that a frame reaches the extension — a throwaway `websocat`-level check, before either client is built. If the key is refused, everything below is blocked and that is a hire change.
      **Closed against live `freehire.dev`.** `Authorization: Bearer fhk_…`, the key already in `~/.freehire/creds.json`, upgraded to `101 Switching Protocols` on `?role=harness`; the frame `{"id":"gate-1","tool":"read_form"}` came back as `{"id":"gate-1","error":"the browser extension is not connected"}` — accepted, routed, and answered on the same id. So the credential, the role and the framing all work as documented, and no hire change is needed.
      Two things this settles for the clients: the handshake **must** be HTTP/1.1 (over HTTP/2 nginx answers `426 Upgrade Required`), and the server's no-extension message is already a sentence a user can act on, so rendering it means not mangling it rather than inventing it.
      Still unverified: a frame reaching the *extension* and returning real fields. That needs a browser with the panel open and is the one part a terminal cannot check.

## 2. freehire-cli — the wire client

- [ ] 2.1 A WebSocket client that connects as `role=harness` with the key from `~/.freehire/creds.json`, sends one call, reads the answer correlated by `id`, and closes. Unit-test the framing and the id correlation against a fake server, not a live one.
- [ ] 2.2 The failure paths, each ending in a sentence the user can act on: no key configured, no extension attached, a timeout above the extension's own widget waiting, a displaced connection.
- [ ] 2.3 An outcome the client does not recognise is printed verbatim rather than mapped onto a familiar one. Unit-test with an invented status.

## 3. freehire-cli — the commands

- [ ] 3.1 `freehire browser read-form` — the page's questions, with `--json` for an agent, matching the repo's existing output convention.
- [ ] 3.2 `freehire browser fill` — one or more label/value pairs, reporting an outcome per requested fill.
- [ ] 3.3 `freehire browser combobox open|options|select|verify` — the four primitives as subcommands, each one call. Verify that opening in one invocation and reading options in the next works against a real page: the design rests on the widget's state belonging to the page, not the connection.
- [ ] 3.4 Help text that says what this cannot do — it does not submit, and it does not filter values against the profile the way freehire's own autofill does.

## 4. freehire-mcp — the same tools, one socket

- [ ] 4.1 The wire client, holding one connection for the server's life, reconnecting after a drop rather than assuming it stays attached (the relay replaces a harness when another joins).
- [ ] 4.2 The six primitives as MCP tools, with the names, arguments and outcomes settled in 1.1 — checked against the CLI's, so a loop written for one runs on the other.
- [ ] 4.3 Tool descriptions that tell the host's model what it is holding: the widget steps are ordered, an unverified write is not a fill, and an ambiguous label is not retried.
- [ ] 4.4 Whatever 1.2 decided about opt-in.

## 5. Verify

- [ ] 5.1 Both repos' own checks: `go build` + `go test` + `go vet` in freehire-cli; build + tests in freehire-mcp.
- [ ] 5.2 Drive one real Greenhouse form end to end through the CLI, and the same form through the MCP server, and confirm the outcomes match call for call. Nothing is submitted.
- [ ] 5.3 The interaction the design warns about: with the MCP harness attached, run a CLI command and confirm the MCP server notices it was displaced and recovers, rather than silently answering nothing.
- [ ] 5.4 Confirm the hosted path still works — the panel's Autofill button, with no harness attached, fills as it did before.
