# OCR Actions Implementation Plan

**Goal:** Review PRs automatically and through `@codereview`, using private gateway settings.

**Architecture:** A metadata job authorizes requests and resolves PR refs; the pinned
OCR CLI reviews and a small runner sanitizes its findings before publication. Only
the review step receives gateway secrets.

**Tech stack:** GitHub Actions, GitHub Script, OpenCodeReview 1.13.9.

**Spec:** [OCR Actions design](../specs/2026-11-13-ocr-actions-design.md).

## Tasks

- [x] Add `.github/workflows/code-review.yml` with automatic, comment and manual triggers.
- [x] Check closed PRs, drafts, unauthorized mentions and unrelated comments are handled.
- [x] Run actionlint, zizmor, Python tests and applicable documentation checks.
- [x] Set the two repository secrets privately from the verified local OCR configuration.
- [ ] Publish a PR and dispatch the workflow against it; inspect the run and its result.

## Review focus

Fork contents must never execute with secrets. Model failures must not publish credentials
or gateway addresses in comments, logs or artifacts. A normal PR comment must not cancel an active review.
The manual trigger must reject invalid PR numbers and resolve current refs through the API.
