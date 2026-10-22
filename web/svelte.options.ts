import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mdsvex } from 'mdsvex';

// How a component compiles, shared by the app build (vite.config.ts → sveltekit()) and
// the component tests (vitest.config.ts → svelte()). Both read it from svelte.config.js
// until SvelteKit 3 retired that file; without one shared copy the tests compile `.svx`
// as plain Svelte and fail on the frontmatter.
//
// `.svelte` compiles as usual; `.svx` blog posts (web/src/posts/*.svx) go through
// mdsvex, which compiles markdown + frontmatter to Svelte components at build time.
// mdsvex output is bundled same-origin, so the strict script-src CSP is unaffected.
export const svelteOptions = {
  extensions: ['.svelte', '.svx'],
  preprocess: [vitePreprocess(), mdsvex({ extensions: ['.svx'] })],
};
