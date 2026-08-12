import js from '@eslint/js';
import ts from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import oxlint from 'eslint-plugin-oxlint';
import globals from 'globals';

export default ts.config(
  // Generated output is not source — never lint it.
  { ignores: ['.output/', '.wxt/', 'node_modules/'] },

  js.configs.recommended,
  ...ts.configs.strict,
  ...svelte.configs.recommended,

  // Extension contexts see both DOM globals (sidepanel, content script) and
  // the WebExtension API surface (chrome/browser, background). Type-aware
  // linting is left to `svelte-check`, so no `projectService` here — ESLint
  // stays syntactic, mirroring web/eslint.config.js.
  {
    files: ['**/*.{ts,svelte,svelte.ts}'],
    languageOptions: {
      globals: { ...globals.browser, ...globals.webextensions },
      parserOptions: {
        extraFileExtensions: ['.svelte'],
        parser: ts.parser,
      },
    },
  },

  // Config files run in Node, not the browser.
  {
    files: ['*.config.{js,ts}', 'wxt.config.ts'],
    languageOptions: { globals: { ...globals.node } },
  },

  // Allow intentionally-unused names prefixed with `_`.
  {
    rules: {
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_' }],
    },
  },

  // Must come last: disables every ESLint rule that oxlint already covers.
  ...oxlint.configs['flat/recommended'],
);
