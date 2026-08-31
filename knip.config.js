// Dead-code and unused-dependency analysis. `pnpm check:dead` runs it, and that script runs
// `svelte-kit sync` first — without the generated $app/* and ./$types modules knip cannot
// resolve half of web/src and reports the failure as dead code.
//
// A .js config rather than knip.json: knip validates the JSON schema strictly and rejects
// `//` keys, so the arguments below would have had to live somewhere nobody reads them.
//
// THE GATE COVERS FILES, DEPENDENCIES AND BINARIES, NOT EXPORTS. That line is drawn by
// signal, and both sides of it were measured. The gated categories produced one finding on
// adoption and it was real: web declared its own copy of tailwind-variants, which only
// design-system imports and which design-system already declares — two copies of one
// library, free to drift, in a package that never used it.
//
// Exports are a different shape here, so `pnpm check:dead:exports` reports them and nothing
// gates them. Eighty-one, and the two large groups are arguments against gating rather than
// a backlog: design-system/src/index.ts is a package's PUBLIC API, where an export with no
// consumer yet is the normal state of a design system — and how many primitives have one is
// already measured, deliberately, by check:adoption. The rest are mostly constants exported
// out of habit and read only inside their own module. Worth removing in reviewed passes;
// not worth a gate that would be argued with on every unrelated change.

export default {
  // Scripts invoked from the CI workflow and from this package's own scripts, which knip
  // reads and cannot resolve: each CI job sets its own working-directory, and `check:dead`
  // reaches svelte-kit through `pnpm --dir web`, so neither binary belongs to the workspace
  // knip is looking in.
  ignoreBinaries: ['build', 'check', 'build-storybook', 'svelte-kit'],

  workspaces: {
    // scripts/*.py is the harvest tooling — Python, invisible to knip either way.
    '.': {
      entry: ['scripts/*.mjs'],
      project: ['scripts/**'],
      // check-migrations.mjs builds a path to node_modules/.bin/squawk, which knip reads as
      // an import of a package called `squawk`. The package is `squawk-cli`, and it is used
      // through that path rather than through an import.
      ignoreDependencies: ['squawk-cli', 'squawk'],
    },

    'design-system': {
      entry: ['src/*.stories.ts', 'scripts/*.mjs'],
      project: ['src/**', 'scripts/**'],
      // Loaded by name from eslint.config.js, never imported.
      ignoreDependencies: ['svelte-eslint-parser'],
    },

    // WXT builds the extension from entrypoints/, so those are the roots — nothing imports
    // them. The two config files are read by tooling rather than by code.
    extension: {
      entry: ['entrypoints/**/*.{ts,svelte,html}', 'wxt.config.ts', 'vitest.config.ts'],
      project: ['entrypoints/**', 'lib/**'],
      ignoreDependencies: ['svelte-eslint-parser'],
    },

    web: {
      entry: [
        'src/routes/**/+*.{ts,js,svelte}',
        'src/hooks.{client,server}.ts',
        // gen-og.mjs reaches this through a runtime string —
        // vite.ssrLoadModule('/src/lib/server/og/brand.ts') — which no static analysis follows.
        'src/lib/server/og/brand.ts',
        'scripts/*.mjs',
        'cluster.js',
      ],
      project: ['src/**', 'scripts/**'],
      // Emitted by cmd/gen-contracts. It exports the whole domain vocabulary whether or not
      // the SPA has a use for each name yet — that is what a generated contract is for, so
      // its exports are not evidence of anything.
      ignore: ['src/lib/generated/**'],
      // @tsconfig/svelte is consumed by tsconfig.json's `extends`, tslib by the compiler.
      ignoreDependencies: ['@tsconfig/svelte', 'svelte-eslint-parser', 'tslib'],
    },
  },
};
