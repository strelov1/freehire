# LinkedIn Native ATS Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make native ATS ownership and recurring crawling replace LinkedIn ownership for external applications.

**Architecture:** Reuse atsboard, boardcatalog and normal ingestion. Add HRsmart as a native provider and a guarded identity transfer path; preserve job identity and history. Classify LinkedIn apply methods explicitly before publishing.

**Tech Stack:** Go, PostgreSQL/sqlc, existing HTML source client and ingest scheduler.

**Spec:** docs/superpowers/specs/2026-11-16-linkedin-native-ats-design.md

## Global Constraints

No hard job deletes. No guessed ATS identities. No credentials in catalogue or logs. No edits to applied migrations. Preserve public URLs and application history. Keep native crawling as the lifecycle owner.

## Review Focus

- Pagination failure must not falsely prove a complete ATS listing.
- Native identity collision must preserve history rather than overwrite another posting.
- External and unknown LinkedIn methods must not pass as Easy Apply.
- An ATS fetch refusal must not transfer source ownership.
- Repeated migration and crawling must be idempotent.

## Tasks

- [x] Add failing HRsmart listing/detail/pagination and atsboard recognition tests in internal/ingest/sources/hrsmart_test.go and internal/ingest/atsboard/hrsmart_test.go.
- [x] Add internal/ingest/sources/hrsmart.go, registry entry, host recognition and provider note. Prove the real Gist posting is returned under native id3735 and boardgistworld.mua.hrdepartment.com.
- [x] Add guarded native identity transfer with database integration tests for preserved id/slug/history, conflicts, queues and repeat runs. Resolve and validate the actual native posting before writing.
- [x] Classify LinkedIn native/external/unknown methods with parser fixtures and pipeline regressions; reuse ATS discovery and board onboarding for external targets. Use public employer Organization.sameAs and observed careers links when the guest hides the destination; queue boards for normal polling and never substitute LinkedIn postings for unresolved external targets.
- [ ] Run required Go/DB checks, isolated real-stack browser acceptance and independent review, then publish, merge, deploy and verify Gist native ownership and normal ATS schedule.
