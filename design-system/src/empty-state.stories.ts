import type { Meta, StoryObj } from '@storybook/svelte';
import EmptyState from './empty-state.svelte';

const meta = {
  title: 'Primitives/EmptyState',
  component: EmptyState,
  tags: ['autodocs'],
  argTypes: {
    variant: { control: 'select', options: ['default', 'muted', 'destructive'] },
  },
} satisfies Meta<typeof EmptyState>;

export default meta;
type Story = StoryObj<typeof meta>;

export const Default: Story = {
  args: { title: 'No results found', description: 'Try adjusting your filters to see more jobs.' },
};
export const Minimal: Story = { args: { title: 'Nothing here yet' } };
export const Muted: Story = { args: { title: 'Nothing here yet', variant: 'muted' } };
export const Destructive: Story = { args: { title: 'Something went wrong.', variant: 'destructive' } };
