import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

// The primitives are authored in TypeScript, so the components need a
// preprocessor before vite-plugin-svelte can compile them for the tests.
export default { preprocess: vitePreprocess() };
