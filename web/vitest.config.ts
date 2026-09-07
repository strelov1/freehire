import { fileURLToPath } from 'node:url';
import { defineConfig } from 'vitest/config';

// A standalone vitest config (not the SvelteKit vite.config) so unit tests run in
// plain Node without loading the SvelteKit plugin or `$app/*` runtime. Nothing
// tested here needs Svelte compilation.
//
// Not loading that plugin also means not getting the `$lib` alias it provides,
// so `$lib/...` has to be resolved here. Without it a module is testable only by
// accident — `i18n/t.ts` imports `$lib/locale` and passes because that import is
// `import type` and erases before the resolver sees it, while `i18n/shell.ts`
// imports a value from `$lib/i18n/t` and fails on load. Which of the two a file
// happens to be is no basis for whether it can have a test.
export default defineConfig({
  resolve: {
    alias: {
      $lib: fileURLToPath(new URL('./src/lib', import.meta.url)),
    },
  },
  test: {
    environment: 'node',
    // `scripts/**` is in the net because the contributors collector is a plain Node
    // script by design — the GitHub Action that runs it daily invokes `node` with no
    // install and no build step, which a TypeScript collector would have cost. Its
    // fetching is untestable either way, but the assembly around it (the per-person
    // totals, the twenty-pull-request cap, the stable key order that decides whether a
    // run commits) is ordinary logic, and this is what keeps it from being exercised
    // only by a nightly job nobody watches.
    include: ['src/**/*.test.ts', 'scripts/**/*.test.mjs'],
  },
});
