import type { StorybookConfig } from '@storybook/svelte-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|js)'],
  addons: ['@storybook/addon-essentials'],
  framework: {
    name: '@storybook/svelte-vite',
    options: {},
  },
  viteFinal: async (config) => {
    // Ensure @sveltejs/vite-plugin-svelte processes ALL .svelte files,
    // including Storybook framework's own components.
    const { svelte } = await import('@sveltejs/vite-plugin-svelte');
    config.plugins = config.plugins || [];
    config.plugins.push(svelte());
    return config;
  },
};

export default config;
