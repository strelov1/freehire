// vite's ambient types are what declare a side-effect CSS import. Referenced
// per-file rather than through tsconfig's `types`, which would also cut the
// package off from every other @types package it picks up automatically.
/// <reference types="vite/client" />
import type { Preview } from '@storybook/svelte';
import { withThemeByClassName } from '@storybook/addon-themes';

// Tailwind plus the package's token/theme contract. Without it the primitives
// render as unstyled markup while the build stays green — see preview.css.
import './preview.css';

const preview: Preview = {
  // The dark tokens are scoped to `.dark` (scripts/build-tokens.mjs builds them
  // under that selector), which is a DOM concern the `backgrounds` toolbar
  // cannot serve — it only paints the canvas, and silently ignores anything else
  // an option carries. So the class is what the toolbar switches, and the canvas
  // follows the tokens from preview.css.
  decorators: [
    withThemeByClassName({
      themes: { light: '', dark: 'dark' },
      defaultTheme: 'light',
      parentSelector: 'html',
    }),
  ],
  parameters: {
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
  },
};

export default preview;
