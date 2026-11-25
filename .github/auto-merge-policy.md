# Auto-merge policy

The SuperPlane canvas "PR auto-merge policy" reads this file from `main` after the ready check
labels a pull request `ready-to-merge`. The principle: a pull request whose pipeline is green and
whose review has nothing left open merges on its own. The owner decided this on 2026-11-24, in
place of the earlier list of zones that always waited for a person.

## Auto-merge

A pull request auto-merges when all of these hold on the head commit being judged:

- The latest `## Ready check` comment's verdict is **Ready** for that commit.
- Every required check is green on that commit.
- No review finding is open: every CodeRabbit and human review comment is resolved, or the
  latest ready check comment names it and says why it was skipped. A review that requests
  changes and was not dismissed counts as open.

## Always a person

- A pull request that changes this file: it would rewrite the rule that judges it.
- A pull request from a fork (the canvas never passes one on).
- A pull request the ready check marked with an open question for the owner.
