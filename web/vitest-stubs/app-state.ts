// `$app/state` is a virtual module the SvelteKit vite plugin provides; the
// `components` vitest project loads only `@sveltejs/vite-plugin-svelte`, not
// SvelteKit itself (see vitest.config.ts), so this alias target exists purely so
// `vi.mock('$app/state', ...)` has a real module id to intercept. A component
// under test always overrides this via `vi.mock`, so its own content is inert.
export const page = { url: new URL('http://localhost/') };
