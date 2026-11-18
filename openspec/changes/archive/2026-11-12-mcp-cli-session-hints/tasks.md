## 1. CLI session instructions

- [x] 1.1 Test (RED): initializing the canonical endpoint with User-Agent `freehire-cli/1.4.0` returns instructions ending with the CLI sentence, signed in and anonymous; `Claude-User`/`openai-mcp`/empty User-Agent get instructions without it; sentence ≤ 256 chars
- [x] 1.2 Implement: detect the CLI User-Agent in `MixedHandler`, pass it to `newMixedServer`, append the sentence to the instructions
- [x] 1.3 Document the CLI session sentence in `internal/api/mcpapp/AGENTS.md`

## 2. Verification

- [x] 2.1 `gofmt -l`, `go vet ./...`, `go vet -tags=integration ./...`, `go test ./internal/api/...` green
