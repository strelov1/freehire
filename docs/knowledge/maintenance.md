---
type: Playbook
title: Maintain this knowledge bundle
description: Update OKF concepts alongside their sources and record evidence honestly.
generated:
  by: codex/gpt-6.1-sol
  at: 2026-11-14T22:00:00-03:00
sources:
  - resource: https://github.com/strelov1/freehire/blob/c0fc3518d5f25c66f39b069c943fe7a1fab840b9/AGENTS.md
---

## Trigger

A change affects a fact or ownership pointer in this bundle, or a reader finds
that a source contradicts a concept.

## Steps

1. Read the affected source and the nearest package instructions.
2. Update the affected concept and its `sources` URLs to the source revision
   actually inspected. Use relative Markdown links for navigation in this checkout.
3. Set `generated.by` to the actual author and `generated.at` to the time of the
   meaningful edit. Record the change in [log.md](log.md).
4. Check YAML frontmatter and local links, then run the repository Markdown gates
   described in [CI checks](../agents/ci-checks.md).

## Evidence and trust

This is an OKF v0.2 bundle: concepts use Markdown with YAML frontmatter and a
required `type`. [The specification](https://github.com/GoogleCloudPlatform/open-knowledge-format/blob/main/SPEC.md)
defines the fields. The source snapshots identify provenance, not proof that
production matches the document. No `verified` field is supplied for this initial
agent-authored bundle. Add one only after the stated actor actually confirms the
content against its sources. Dates in `generated` describe edits, not verification.

Keep detailed rules at their existing source. Add a concept when it answers a
concrete recurring question; give it an entry in [index.md](index.md). Resolve
conflicting sources explicitly rather than merging their claims silently.
