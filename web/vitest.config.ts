import { fileURLToPath } from 'node:url';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vitest/config';

// A standalone vitest config (not the SvelteKit vite.config), split into two
// projects with nothing in common — same shape as design-system/vitest.config.ts.
//
// `unit` runs in plain Node without loading the SvelteKit plugin or `$app/*`
// runtime, because nothing in it needs Svelte compilation. Not loading that
// plugin also means not getting the `$lib` alias it provides, so `$lib/...` has
// to be resolved here. Without it a module is testable only by accident —
// `i18n/t.ts` imports `$lib/locale` and passes because that import is `import
// type` and erases before the resolver sees it, while `i18n/shell.ts` imports a
// value from `$lib/i18n/t` and fails on load. Which of the two a file happens to
// be is no basis for whether it can have a test.
//
// `components` renders actual `.svelte` files (`@testing-library/svelte` +
// jsdom) for behavior that only exists inside a component's event handlers —
// e.g. SkillsCard's autosave-then-notify wiring. It is a distinct `*.spec.ts`
// extension, not `*.test.ts`, so it can never accidentally pick up one of
// `unit`'s 100+ existing files under the Svelte/browser-condition resolver they
// were never written against.
const alias = { $lib: fileURLToPath(new URL('./src/lib', import.meta.url)) };

// SvelteKit's `$app/*` are virtual modules the SvelteKit vite plugin provides;
// `components` loads only `@sveltejs/vite-plugin-svelte`, not the SvelteKit
// plugin. A component under test that imports one always overrides it with its
// own `vi.mock`, so these aliases exist only to give `vi.mock` a resolvable
// module id — see vitest-stubs/*.ts.
const appStubAlias = {
  '$app/state': fileURLToPath(new URL('./vitest-stubs/app-state.ts', import.meta.url)),
  '$app/paths': fileURLToPath(new URL('./vitest-stubs/app-paths.ts', import.meta.url)),
  '$app/navigation': fileURLToPath(new URL('./vitest-stubs/app-navigation.ts', import.meta.url)),
};

export default defineConfig({
  test: {
    projects: [
      {
        resolve: { alias },
        test: {
          name: 'unit',
          environment: 'node',
          // `scripts/**` is in the net because the contributors collector is a plain
          // Node script by design — the GitHub Action that runs it daily invokes
          // `node` with no install and no build step, which a TypeScript collector
          // would have cost. Its fetching is untestable either way, but the assembly
          // around it (the per-person totals, the twenty-pull-request cap, the
          // stable key order that decides whether a run commits) is ordinary logic,
          // and this is what keeps it from being exercised only by a nightly job
          // nobody watches.
          include: ['src/**/*.test.ts', 'scripts/**/*.test.mjs'],
        },
      },
      {
        resolve: { alias: { ...alias, ...appStubAlias }, conditions: ['browser'] },
        plugins: [svelte()],
        test: {
          name: 'components',
          environment: 'jsdom',
          include: ['src/**/*.spec.ts'],
          setupFiles: ['./vitest.setup.ts'],
        },
      },
    ],
  },
});
