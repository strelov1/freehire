import js from '@eslint/js';
import ts from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import oxlint from 'eslint-plugin-oxlint';
import globals from 'globals';

export default ts.config(
  // Generated output is not source — never lint it. `.svelte-kit/` holds the
  // SvelteKit-synced types and the build artifacts, which otherwise flood the
  // report with no-undef/no-explicit-any from machine-written code.
  { ignores: ['dist/', 'node_modules/', '.svelte-kit/', 'build/'] },

  js.configs.recommended,
  ...ts.configs.strict,
  ...svelte.configs.recommended,

  // Browser globals for app code; the `.svelte` parser needs the TS parser to
  // understand `<script lang="ts">` and rune modules. Type-aware linting is left
  // to `svelte-check`, so no `projectService` here — ESLint stays syntactic.
  {
    files: ['**/*.{ts,svelte,svelte.ts}'],
    languageOptions: {
      globals: { ...globals.browser },
      parserOptions: {
        extraFileExtensions: ['.svelte'],
        parser: ts.parser,
      },
    },
  },

  // Config files run in Node, not the browser.
  {
    files: ['*.config.{js,ts}'],
    languageOptions: { globals: { ...globals.node } },
  },

  // Standalone scripts (codegen, smoke tests) run in Node, not the browser.
  // cluster.js is the production entry point systemd starts — same Node globals,
  // but it belongs beside build/ rather than under scripts/.
  {
    files: ['scripts/**/*.{js,mjs}', 'cluster.js'],
    languageOptions: { globals: { ...globals.node } },
  },

  // Allow intentionally-unused names prefixed with `_` (e.g. `{#each xs as _, i}`).
  {
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    },
  },

  // The change-array-by-copy methods reach the browser untranspiled — they are
  // runtime methods, so no build target polyfills them, and Safari only grew them
  // in 16.4. One `.toSorted()` evaluated while `facets.ts` was initialising threw
  // on iOS 15, which takes the module down and with it the whole page — not the
  // one sorted list. Every call we had ran on an array a `.map`/`.filter`/spread
  // had just produced, so `.sort()` in place is the same thing without the floor
  // on who can open the site. Lift this once the analytics say iOS 15 is gone.
  {
    files: ['**/*.{ts,svelte,svelte.ts}'],
    ignores: ['**/*.test.ts', 'scripts/**', 'src/lib/server/**'],
    rules: {
      'no-restricted-syntax': [
        'error',
        {
          selector:
            "MemberExpression[property.name=/^(toSorted|toReversed|toSpliced)$/][computed=false]",
          message:
            'Not in Safari before 16.4, and a throw here kills the whole module. Use [...x].sort() — or .sort() when the array is already a fresh copy.',
        },
      ],
    },
  },

  // Must come last: disables every ESLint rule that oxlint already covers.
  ...oxlint.configs['flat/recommended'],
);
