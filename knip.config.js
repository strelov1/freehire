// Dead-code and unused-dependency analysis. `pnpm check:dead` runs it, and that script runs
// `svelte-kit sync` first — without the generated $app/* and ./$types modules knip cannot
// resolve half of web/src and reports the failure as dead code.
//
// A .js config rather than knip.json: knip validates the JSON schema strictly and rejects
// `//` keys, so the arguments below would have had to live somewhere nobody reads them.
//
// THE GATE COVERS EVERYTHING KNIP REPORTS, INCLUDING EXPORTS, and it took a cleanup to get
// there. The first run found 81 unused exports, which is the number that usually turns into
// "report them, do not gate them". Working through them left 13, and every one was
// deliberate rather than forgotten — so the argument moved out of this file and next to the
// code, as a JSDoc `@public` tag with the reason beside it. That is the better home for it:
// a config-level exemption is invisible from the line it excuses.
//
// The two shapes that earned a tag:
//
//   web/src/lib/types.ts, cv.ts and collections.ts re-export the generated contract under
//   the app's own names. A name is carried there whether or not a screen reads it yet —
//   that is what the facade is for, and a missing one is the bug it prevents.
//
//   stages.ts exports a compile-time assertion. Declaring it IS the check; `export` is only
//   what stops the linter deleting it as an unused local.
//
// Everything else was a real finding. 50 exports were read only inside their own module and
// simply lost the keyword; that in turn let eslint see three genuinely dead declarations it
// could not see through an export, which is worth knowing — knip and no-unused-vars answer
// different halves of one question, and neither is complete alone.

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
      entry: ['entrypoints/**/*.{ts,svelte,html}'],
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
