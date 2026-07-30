import type { Meta, StoryObj } from '@storybook/svelte';
import TooltipDemo from '../.storybook/demos/TooltipDemo.svelte';

const meta = {
  title: 'Primitives/Tooltip',
  component: TooltipDemo,
  tags: ['autodocs'],
  argTypes: {
    side: { control: 'select', options: ['top', 'right', 'bottom', 'left'] },
  },
} satisfies Meta<typeof TooltipDemo>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Top: Story = { args: { side: 'top' } };
export const Right: Story = { args: { side: 'right' } };
export const Bottom: Story = { args: { side: 'bottom' } };
export const Left: Story = { args: { side: 'left' } };
// max-w-xs wraps rather than stretching the trigger's row.
export const LongContent: Story = {
  args: {
    side: 'top',
    label:
      'Reposted three times in six months with no change to the description — the pattern the ghost-job signal looks for.',
  },
};
