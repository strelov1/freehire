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

  // Built-ins newer than our floor reach the browser untranspiled — they are runtime
  // methods, so no build target polyfills them. One `.toSorted()` evaluated while
  // `facets.ts` was initialising threw on iOS 15, which takes the module down and with
  // it the whole page — not the one sorted list. `Object.hasOwn` then did the same to
  // the header search's suggestions, which is why the rule names a class rather than
  // the one method that got us. Add to it when the next one is found in the wild, and
  // lift the whole block once the analytics say iOS 15 is gone.
  {
    files: ['**/*.{ts,svelte,svelte.ts}'],
    ignores: ['**/*.test.ts', 'scripts/**', 'src/lib/server/**'],
    rules: {
      'no-restricted-syntax': [
        'error',
        {
          // Change-array-by-copy: Safari 16.4. Every call we had ran on an array a
          // `.map`/`.filter`/spread had just produced, so sorting in place is the
          // same thing without the floor on who can open the site.
          selector:
            "MemberExpression[property.name=/^(toSorted|toReversed|toSpliced)$/][computed=false]",
          message:
            'Not in Safari before 16.4, and a throw here kills the whole module. Use [...x].sort() — or .sort() when the array is already a fresh copy.',
        },
        {
          // Safari 15.4. Every use we had asked about decoded JSON, where a present
          // key always holds a value, so reading the value answers the same question.
          selector: "MemberExpression[object.name='Object'][property.name='hasOwn']",
          message:
            'Not in Safari before 15.4, and a throw here kills the whole module. Use `obj[k] !== undefined`, or `Object.prototype.hasOwnProperty.call(obj, k)` when a key holding undefined must still count.',
        },
      ],
    },
  },

  // Must come last: disables every ESLint rule that oxlint already covers.
  ...oxlint.configs['flat/recommended'],
);
