import type { Meta, StoryObj } from '@storybook/svelte';
// A separate entry rather than a story inside button.stories.ts: the icon size
// needs a component for its child, which makes the meta a different type.
import IconButtonDemo from '../.storybook/demos/IconButtonDemo.svelte';

const meta = {
  title: 'Primitives/Button (icon size)',
  component: IconButtonDemo,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['primary', 'secondary', 'outline', 'ghost'] },
  },
} satisfies Meta<typeof IconButtonDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Ghost: Story = { args: { variant: 'ghost' } };
export const Outline: Story = { args: { variant: 'outline' } };
