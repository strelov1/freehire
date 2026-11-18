// Conventional Commits for pull request titles. Merges to main are squash merges whose
// commit message is the PR title, so the title is the commit that history, changelogs
// and filters read. Checked in .github/workflows/pr-title.yml and, for local commits,
// by the commit-msg hook in lefthook.yml.
export default {
  extends: ['@commitlint/config-conventional'],
  // Without this a title like `Revert "fix: x"` is skipped by commitlint's built-in ignores.
  defaultIgnores: false,
  rules: {
    // The format is `type(scope)?: summary` and nothing more: the preset's case rule would
    // reject `fix: Close empty boards`, and its 100-character cap a long but valid title.
    'subject-case': [0],
    'header-max-length': [0],
    'type-enum': [
      2,
      'always',
      ['feat', 'fix', 'perf', 'refactor', 'docs', 'test', 'ci', 'build', 'chore', 'revert'],
    ],
  },
};
