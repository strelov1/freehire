# Source ingest conventions

Before editing or reviewing this area, read the applicable sections of
[local requirements](AGENT-REFERENCE.md). Read the complete reference when changing
architecture, contracts, authorization, state transitions or cross-module behavior.
The reference retains all requirements previously held here.

## Package layout (moving, #4586)

`sources/kit` is the adapter kernel: `Source`, `Job`, `CompanyEntry`, the HTTP client interfaces,
`StatusError` and the shared parsing helpers. It imports nothing of `sources`. Adapters are
moving out of `sources` into family subpackages that import only `kit`; `sources` keeps the one
registry (`All`) and the classifiers that read the capability markers (`markers.go`). Until the
last adapter leaves, `kit_aliases.go` re-exports the kernel under its old names; new adapter
code in a family calls `kit.X` directly. A marker is a method with an exported name
(`FullBoardListing()`), so a family adapter can declare it.

Families so far: [`sources/ats`](ats/AGENTS.md) (greenhouse, lever, ashby, recruitee). A family
imports only `kit` (and `applyform`) of ingest; `.golangci.yml` rule `sources-families` holds it.

## Scope

Read [Scope](AGENT-REFERENCE.md#scope) before work involving this topic.

## Provider notes

Read [Provider notes](AGENT-REFERENCE.md#provider-notes) before work involving this topic.

## Always true

Read [Always true](AGENT-REFERENCE.md#always-true) before work involving this topic.

## How it works

Read [How it works](AGENT-REFERENCE.md#how-it-works) before work involving this topic.

## Recent-only crawls (`RecentSource`)

Read [Recent-only crawls (`RecentSource`)](AGENT-REFERENCE.md#recent-only-crawls-recentsource) before work involving this topic.

## Limitations

Read [Limitations](AGENT-REFERENCE.md#limitations) before work involving this topic.

## A crawl that reads nothing of what it listed must fail

Read [A crawl that reads nothing of what it listed must fail](AGENT-REFERENCE.md#a-crawl-that-reads-nothing-of-what-it-listed-must-fail) before work involving this topic.

## The Scrapling tier (the sources no plain client reaches)

Read [The Scrapling tier (the sources no plain client reaches)](AGENT-REFERENCE.md#the-scrapling-tier-the-sources-no-plain-client-reaches) before work involving this topic.
