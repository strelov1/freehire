import js from '@eslint/js';
import ts from 'typescript-eslint';
import svelte from 'eslint-plugin-svelte';
import oxlint from 'eslint-plugin-oxlint';
import globals from 'globals';

export default ts.config(
  // Generated output is not source — never lint it.
  { ignores: ['dist/', 'node_modules/', 'storybook-static/'] },

  js.configs.recommended,
  ...ts.configs.strict,
  ...svelte.configs.recommended,

  // Component/token source runs in the browser (or a Storybook preview
  // iframe, same environment). Type-aware linting is left to `svelte-check`,
  // so no `projectService` here — ESLint stays syntactic, mirroring
  // web/eslint.config.js.
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

  // Config files, the token build, and Storybook's own config run in Node.
  {
    files: ['*.config.{js,ts}', 'scripts/**/*.{js,mjs}', '.storybook/main.ts'],
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
