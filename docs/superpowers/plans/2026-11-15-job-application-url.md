# Job Application URL Implementation Plan

> **For agentic workers:** Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Open verified employer destinations directly without changing posting provenance.

**Architecture:** Add an independently maintained application URL column, project it at
the public wire boundary, and provide a bounded curator command with atomic reindex enqueue.

**Tech Stack:** Go, PostgreSQL, sqlc; existing public JSON and Svelte links.

**Spec:** [Approved design](../specs/2026-11-15-job-application-url-design.md).

## Global Constraints

Keep source URLs, lifecycle and source attribution unchanged. No authenticated crawl.
Never edit applied migrations. Preserve known destinations through ingest.

## Tasks

- [ ] Add a failing projection regression for the separate destination and fallback.
- [ ] Add the expansive migration and regenerate sqlc; map the field into public jobs
      and copy links while preserving the source URL internally.
- [ ] Add a validated curator command and transactional update/enqueue query; cover
      validation, clearing, missing/private rows, repeat updates and crawl persistence.
- [ ] Update provider/operator notes, run required checks, independently review and fix.
- [ ] Publish, merge and deploy through the normal workflow; set the verified Gist URL
      and verify the deployed page and search destination.
