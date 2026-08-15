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

  // Must come last: disables every ESLint rule that oxlint already covers.
  ...oxlint.configs['flat/recommended'],
);
