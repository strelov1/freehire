## 1. Shard primitives

- [x] 1.1 Add `sources.ParseShard("i/n")` (validates 1<=i<=n, n>=1) and `Config.Shard(i, n)` (round-robin slice, preserves provider); unit-test parsing, the round-robin partition, complete-no-overlap coverage, and the n<=1 passthrough.

## 2. Wire the ingest command

- [x] 2.1 Parse `--shard=i/n` (or `SHARD` env) in `cmd/ingest`, apply `Config.Shard` after validation and before the crawl, and log the slice; a malformed selector fails fast.

## 3. Verify

- [x] 3.1 `go build/vet/test ./...`; smoke the binary: invalid shard exits fast, a valid shard logs "crawling N of M boards" and crawls only its slice.

## 4. Ops (freehire-ops, applied at deploy)

- [ ] 4.1 Replace the hourly `freehire-ingest@workday` timer with 6 staggered shard timers (`--shard=1/6`..`6/6`, every 6h, one per hour); disable the old workday timer.
