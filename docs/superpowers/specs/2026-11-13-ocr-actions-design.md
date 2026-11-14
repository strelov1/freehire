# OCR pull request review

Run OpenCodeReview automatically on open, non-draft PRs and new commits, and on a
standalone `@codereview` mention from an owner, member or collaborator. Publish inline
findings and an updated summary on the PR. Manual dispatch accepts a PR number.

## Approach

Use the official action, pinned to v1.13.9's commit, rather than maintaining a custom
CLI installer and comment publisher. A metadata job resolves the open PR first; only
valid requests enter the review's per-PR concurrency group. Unrelated comments must
never cancel reviews. The action checks out trusted base code and reads PR git objects.

The gateway endpoint and credential live exclusively in Actions secrets
`OCR_LLM_URL` and `OCR_LLM_AUTH_TOKEN`. `OCR_LLM_MODEL` is an optional Actions variable
(default `mid`). Use the OpenAI protocol and the gateway virtual-key header. Disable
raw artifacts and streamed logs. No gateway address or credential belongs in Git.

## Validation

Lint the workflow with actionlint and zizmor, exercise mention and PR eligibility,
and dispatch a real review from this branch before claiming the integration works.
Automatic and comment triggers become active when the workflow reaches the default branch.
