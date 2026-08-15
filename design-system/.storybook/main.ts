import type { StorybookConfig } from '@storybook/svelte-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|js)'],
  // Serves static/email-previews/, the HTML the Go mail templates render into
  // (`go run ./cmd/mail-preview`). Storybook can only frame files under its own
  // root, so the generator writes them here rather than beside the Go code.
  staticDirs: ['../static'],
  // No addon-essentials: Storybook folded it into core, and it was never
  // published past 8.6. addon-themes is separate and still shipped — it owns the
  // toolbar that puts `.dark` on the preview root, which is where the dark
  // tokens live.
  addons: ['@storybook/addon-themes'],
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
    // The primitives are styled entirely through Tailwind utilities, so the
    // preview needs the same compiler web's vite config runs.
    const { default: tailwindcss } = await import('@tailwindcss/vite');
    config.plugins = config.plugins ?? [];
    config.plugins.push(svelte(), tailwindcss());
    return config;
  },
};

export default config;
