import type { Preview } from '@storybook/svelte';

// Import the generated tokens so stories render with the correct light/dark values.
import 'freehire-design-system/dist/tokens-light.css';

const preview: Preview = {
  parameters: {
    controls: { matchers: { color: /(background|color)$/i, date: /Date$/i } },
    backgrounds: {
      options: {
        light: { name: 'Light', value: 'oklch(0.997 0.003 130)' },
        dark: { name: 'Dark', value: 'oklch(0.16 0.006 110)', class: 'dark' },
      },
    },
  },
};

export default preview;
