import type { StorybookConfig } from '@storybook/svelte-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|js)'],
  // No addon-essentials: Storybook folded it into core, and it was never
  // published past 8.6.
  framework: {
    name: '@storybook/svelte-vite',
    // Storybook's svelte docgen plugin hands the raw .svelte source to the
    // bundler's JS parser, which fails on the markup — on rollup and rolldown
    // alike. It buys the autodocs prop tables; losing them is the cheaper side
    // of the trade against pinning the whole package off the vite the rest of
    // the repo is on. Revisit when the plugin runs after the svelte transform.
    options: { docgen: false },
  },
  viteFinal: async (config) => {
    // Storybook ships its own .svelte components (PreviewRender), and the
    // framework's plugin does not reach them — without a second svelte()
    // instance the build hands them to the JS parser and dies on the markup.
    const { svelte } = await import('@sveltejs/vite-plugin-svelte');
    config.plugins = config.plugins ?? [];
    config.plugins.push(svelte());
    return config;
  },
};

export default config;
