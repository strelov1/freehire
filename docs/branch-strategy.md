# Branch strategy

`main` is the single source of truth. It is the base branch for new work and the merge
target for everything: pull/rebase your branch from `origin/main`, and PRs land on `main`.
When the `conflict-resolver` skill needs to know "which branch has the latest changes,"
this file is the answer for this repo — that answer varies per project, which is why it's
called out here instead of assumed.

There is currently no enforced branch-naming convention. History shows a mix of styles in
active use — `feat/x`, `fix/x`, `ci/x`, `extension/x`, and plain descriptive names like
`add-sources-validator` or `main-feedback-fix` — and none of them is required over another.
Name a branch descriptively; that's the only rule right now.

<!--
Optional stricter convention, not currently adopted — revisit if the freeform naming above
ever becomes a real problem (e.g. hard to tell what a branch does from CI/PR lists):

- `feat/<short-slug>` — new functionality
- `fix/<short-slug>` — bug fixes
- `chore/<short-slug>` — tooling, deps, non-behavioral cleanup

If adopted, update this file to state it as the active rule and remove this comment block.
-->
