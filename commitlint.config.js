// Conventional Commits for pull request titles. Merges to main are squash merges whose
// commit message is the PR title, so the title is the commit that history, changelogs
// and filters read. Checked in .github/workflows/pr-title.yml and, for local commits,
// by the commit-msg hook in lefthook.yml.
export default {
  extends: ['@commitlint/config-conventional'],
  rules: {
    'type-enum': [
      2,
      'always',
      ['feat', 'fix', 'perf', 'refactor', 'docs', 'test', 'ci', 'build', 'chore', 'revert'],
    ],
  },
};
